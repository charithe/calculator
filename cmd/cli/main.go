package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"log"
	"os"

	"github.com/charithe/calculator/pkg/calculator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	app = kingpin.New("Calculator CLI", "A toy RPC calculator CLI")

	addr      = app.Flag("addr", "Server address").Default("localhost:8080").String()
	insecure  = app.Flag("insecure", "Trust unknown CAs").Bool()
	plaintext = app.Flag("plaintext", "Use unencrypted connection").Bool()

	streamCmd = app.Command("stream", "Stream mode")
	batchCmd  = app.Command("batch", "Batch mode")
	batchExpr = batchCmd.Arg("expr", "Expression (space separated)").Strings()
)

func main() {
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case streamCmd.FullCommand():
		doStream()
	case batchCmd.FullCommand():
		doBatch()
	}
}

func doStream() {
	client, err := createClient()
	if err != nil {
		log.Printf("Failed to connect to server: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	log.Printf("Enter each operator or operand in a new line. Press Ctrl+D to end")

	tokChan := make(chan string)
	go func() {
		defer close(tokChan)

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			tokChan <- scanner.Text()
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Failed to read stream: %v", err)
		}
	}()

	result, err := client.EvaluateStream(tokChan)
	if err != nil {
		log.Printf("Streaming call failed: %v", err)
		os.Exit(1)
	}

	log.Printf("Result: %f", result)
}

func doBatch() {
	client, err := createClient()
	if err != nil {
		log.Printf("Failed to connect to server: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	result, err := client.EvaluateBatch(context.Background(), *batchExpr)
	if err != nil {
		log.Printf("Batch call failed: %v", err)
		os.Exit(1)
	}

	log.Printf("Result: %f", result)
}

func createClient() (*calculator.Client, error) {
	var dialOpts []grpc.DialOption
	if *plaintext {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	} else {
		tlsConf := &tls.Config{
			InsecureSkipVerify: *insecure,
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	}

	conn, err := grpc.Dial(*addr, dialOpts...)
	if err != nil {
		return nil, err
	}

	return calculator.NewClient(conn), nil
}
