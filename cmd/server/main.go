package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"time"

	"github.com/charithe/calculator/pkg/calculator"
	"github.com/charithe/calculator/pkg/v1pb"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	isatty "github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	prom "github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/zpages"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/channelz/service"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"gopkg.in/alecthomas/kingpin.v2"
)

const httpTimeout = 5 * time.Second

var (
	app = kingpin.New("Calculator Server", "A toy RPC calculator server")

	debug      = app.Flag("debug", "Enable debug mode").Envar("CALC_DEBUG").Bool()
	listenAddr = app.Flag("listen_addr", "Listen address").Default(":8080").Envar("CALC_LISTEN_ADDR").String()
	logLevel   = app.Flag("log_level", "Log level").Default("info").Envar("CALC_LOG_LEVEL").Enum("error", "warn", "info", "debug")
	statusAddr = app.Flag("status_addr", "Status address").Default(":5000").Envar("CALC_STATUS_ADDR").String()
	tlsCA      = app.Flag("tls_ca", "Path to TLS CA certificate").Envar("CALC_TLS_CA").ExistingFile()
	tlsCert    = app.Flag("tls_cert", "Path to TLS certificate").Envar("CALC_TLS_CERT").ExistingFile()
	tlsKey     = app.Flag("tls_key", "Path to TLS key").Envar("CALC_TLS_KEY").ExistingFile()
)

func main() {
	_ = kingpin.MustParse(app.Parse(os.Args[1:]))

	initLogging()
	startServer()
}

func initLogging() {
	var logger *zap.Logger
	var err error
	errorPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})

	minLogLevel := zapcore.InfoLevel
	switch *logLevel {
	case "debug":
		minLogLevel = zapcore.DebugLevel
	case "info":
		minLogLevel = zapcore.InfoLevel
	case "warn":
		minLogLevel = zapcore.WarnLevel
	case "error":
		minLogLevel = zapcore.ErrorLevel
	}

	infoPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.ErrorLevel && lvl >= minLogLevel
	})

	consoleErrors := zapcore.Lock(os.Stderr)
	consoleInfo := zapcore.Lock(os.Stdout)

	var consoleEncoder zapcore.Encoder
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		encoderConf := zap.NewProductionEncoderConfig()
		encoderConf.MessageKey = "message"
		encoderConf.EncodeTime = zapcore.TimeEncoder(zapcore.ISO8601TimeEncoder)
		consoleEncoder = zapcore.NewJSONEncoder(encoderConf)
	} else {
		encoderConf := zap.NewDevelopmentEncoderConfig()
		encoderConf.EncodeLevel = zapcore.CapitalColorLevelEncoder
		consoleEncoder = zapcore.NewConsoleEncoder(encoderConf)
	}

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleErrors, errorPriority),
		zapcore.NewCore(consoleEncoder, consoleInfo, infoPriority),
	)

	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}

	stackTraceEnabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl > zapcore.ErrorLevel
	})
	logger = zap.New(core, zap.Fields(zap.String("host", host)), zap.AddStacktrace(stackTraceEnabler))

	if err != nil {
		zap.S().Fatalw("Failed to create logger", "error", err)
	}

	zap.ReplaceGlobals(logger.Named("app"))
	zap.RedirectStdLog(logger.Named("stdlog"))
}

func startServer() {
	promExporter, err := initOCPromExporter()
	if err != nil {
		zap.S().Fatalw("Failed to create OpenCensus exporter", "error", err)
	}

	grpcListener, httpListener := startListeners()
	grpcServer := startGRPCServer(grpcListener, calculator.NewService())
	statusServer := startHTTPServer(httpListener, promExporter)

	// await interruption
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt)
	<-shutdownChan

	zap.S().Info("Shutting down")
	grpcServer.GracefulStop()

	ctx, cancelFunc := context.WithTimeout(context.Background(), httpTimeout)
	defer cancelFunc()
	statusServer.Shutdown(ctx)
}

func initOCPromExporter() (*prometheus.Exporter, error) {
	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return nil, err
	}

	registry, ok := prom.DefaultRegisterer.(*prom.Registry)
	if !ok {
		zap.S().Warn("Unable to obtain default Prometheus registry. Creating new one.")
		registry = nil
	}

	exporter, err := prometheus.NewExporter(prometheus.Options{Registry: registry})
	if err != nil {
		return nil, err
	}

	view.RegisterExporter(exporter)
	view.SetReportingPeriod(15 * time.Second)

	return exporter, nil
}

func startListeners() (net.Listener, net.Listener) {
	grpcListener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		zap.S().Fatalw("Failed to create grpc listener", "error", err)
	}

	if *tlsKey != "" && *tlsCert != "" {
		zap.S().Info("Configuring TLS")
		tlsConf, err := getTLSConfig()
		if err != nil {
			zap.S().Fatalw("Failed to configure TLS", "error", err)
		}

		grpcListener = tls.NewListener(grpcListener, tlsConf)
	}

	httpListener, err := net.Listen("tcp", *statusAddr)
	if err != nil {
		zap.S().Fatalw("Failed to create http listener")
	}

	return grpcListener, httpListener
}

func getTLSConfig() (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to load server key pair")
	}

	tlsConfig := defaultTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{certificate}

	if *tlsCA != "" {
		certPool := x509.NewCertPool()
		bs, err := ioutil.ReadFile(*tlsCA)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to read CA certificate")
		}

		ok := certPool.AppendCertsFromPEM(bs)
		if !ok {
			return nil, errors.Wrap(err, "Failed to add CA certificate to pool")
		}

		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		tlsConfig.ClientCAs = certPool
	}

	return tlsConfig, nil
}

func defaultTLSConfig() *tls.Config {
	// See https://blog.cloudflare.com/exposing-go-on-the-internet/
	return &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos: []string{"h2"},
	}
}

func startGRPCServer(listener net.Listener, svc *calculator.Service) *grpc.Server {
	grpc.EnableTracing = true
	grpcLogger := zap.L().Named("grpc")

	codeToLevel := grpc_zap.CodeToLevel(func(code codes.Code) zapcore.Level {
		if code == codes.OK {
			return zapcore.DebugLevel
		}
		return grpc_zap.DefaultCodeToLevel(code)
	})

	serverOpts := []grpc.ServerOption{
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
		grpc_middleware.WithUnaryServerChain(
			grpc_ctxtags.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(grpcLogger, grpc_zap.WithLevels(codeToLevel)),
		),
		grpc_middleware.WithStreamServerChain(
			grpc_ctxtags.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(grpcLogger, grpc_zap.WithLevels(codeToLevel)),
		),
	}

	grpcServer := grpc.NewServer(serverOpts...)

	v1pb.RegisterCalculatorServer(grpcServer, svc)
	healthpb.RegisterHealthServer(grpcServer, svc)

	reflection.Register(grpcServer)
	service.RegisterChannelzServiceToServer(grpcServer)

	go func() {
		zap.S().Infow("Starting grpc server", "addr", *listenAddr)
		if err := grpcServer.Serve(listener); err != nil {
			zap.S().Fatalw("grpc server failed", "error", err)
		}
	}()

	return grpcServer
}

func startHTTPServer(listener net.Listener, promExporter *prometheus.Exporter) *http.Server {
	logger := zap.L().Named("http")

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			io.Copy(ioutil.Discard, r.Body)
			r.Body.Close()
		}
		// TODO use health client
		io.WriteString(w, "OK")
	})
	mux.Handle("/metrics", promExporter)

	if *debug {
		mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		mux.Handle("/debug/", http.StripPrefix("/debug", zpages.Handler))
	}

	httpServer := &http.Server{
		Handler:           mux,
		ErrorLog:          zap.NewStdLog(logger),
		ReadHeaderTimeout: httpTimeout,
		WriteTimeout:      httpTimeout,
		IdleTimeout:       httpTimeout,
	}

	go func() {
		zap.S().Infow("Starting HTTP server", "addr", *statusAddr)
		if err := httpServer.Serve(listener); err != http.ErrServerClosed {
			zap.S().Fatalw("Failed to start HTTP server", "error", err)
		}
	}()

	return httpServer
}
