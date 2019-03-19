gRPC Calculator Demo
=====================

A toy project to demonstrate how a gRPC service might be developed and deployed to Kubernetes.

Requirements:

- Go 1.12 or above
- Docker
- MiniKube or Micro.k8s
- [Prototool](https://github.com/uber/prototool)
- [Skaffold](https://skaffold.dev/)
- [Helm](https://helm.sh/)


Introduction
------------

The gRPC service defined in [pkg/v1pb/calculator.proto](pkg/v1pb/calculator.proto) exposes two RPC endpoints for 
evaluating a postfix mathematical expression. `EvaluateStream` RPC expects to receive a stream of operands and 
operators from the client and returns the result when the stream terminates. `EvaluateBatch` RPC expects the receive
the set of operators and operands as a batch and returns the result of evaluating that batch.

Building
---------

### Server

Run `make build` to generate the gRPC code and build a minimal container image containing the server component.

### CLI

Run `make cli` to produce a local binary named `cli` that can be used to interact with the server


Deploying
---------

### Configuration

The service can be configured via command line flags or environment variables

```
  --debug                Enable debug mode (CALC_DEBUG)
  --listen_addr=":8080"  Listen address (CALC_LISTEN_ADDR)
  --log_level=info       Log level (CALC_LOG_LEVEL)
  --status_addr=":5000"  Status address (CALC_STATUS_ADDR)
  --tls_ca=TLS_CA        Path to TLS CA certificate (CALC_TLS_CA)
  --tls_cert=TLS_CERT    Path to TLS certificate (CALC_TLS_CERT)
  --tls_key=TLS_KEY      Path to TLS key (CALC_TLS_KEY)
```

### Cluster Deployment

If Minikube or any other Kubernetes cluster is accessible and has a working Helm installation, running `make deploy`
will deploy the server to the cluster. As this is a demo project, no ingress is created by the deployment and accessing
the service requires port forwarding as follows:

```
export POD_NAME=$(kubectl get pods --namespace default -l "app.kubernetes.io/name=calculator,app.kubernetes.io/instance=calculator" -o jsonpath="{.items[0].metadata.name}")
kubectl port-forward $POD_NAME 8080:8080 5000:5000
```

The gRPC service is available over port 8080 and can be accessed using the CLI or tools 
such as `grpcurl`. An HTTP status service is available over port 5000 and can be used for health checking (`/status`) 
and obtaining metrics (`/metrics`). If the service was started in debug mode, the HTTP service also exposes profiling 
and monitoring pages as well.


Using the CLI
-------------

Build the CLI by running `make cli`. This will produce a binary named `cli`. The following flags can be used to 
configure the CLI session:

```
Flags:
  --help                   Show context-sensitive help (also try --help-long and --help-man).
  --addr="localhost:8080"  Server address
  --insecure               Trust unknown CAs
  --plaintext              Use unencrypted connection

Commands:
  help [<command>...]
    Show help.

  stream
    Stream mode

  batch [<expr>...]
    Batch mode
```

### Stream Mode

Start the stream mode as follows and then enter each operator and operand in a new line. Press Ctrl+D to calculate
the result.

```
./cli --addr=localhost:8080 --plaintext stream
```

### Batch Mode

Enter the set of operators and operands as space separated arguments

```
./cli --addr=localhost:8080 --plaintext batch 5 10 + 3 '*' 4 +
```
