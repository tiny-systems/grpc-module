# TinySystems gRPC Module

gRPC client component with reflection-based service discovery.

## Components

| Component | Description |
|-----------|-------------|
| gRPC Call | Make gRPC requests using server reflection to discover services, methods, and message schemas automatically |

## Installation

```shell
helm repo add tinysystems https://tiny-systems.github.io/module/
helm install grpc-module tinysystems/tinysystems-operator \
  --set controllerManager.manager.image.repository=ghcr.io/tiny-systems/grpc-module
```

## Run locally

```shell
go run cmd/main.go run --name=grpc-module --namespace=tinysystems --version=1.0.0
```

## Part of TinySystems

This module is part of the [TinySystems](https://github.com/tiny-systems) platform -- a visual flow-based automation engine running on Kubernetes.

## License

This module's source code is MIT-licensed. It depends on the [TinySystems Module SDK](https://github.com/tiny-systems/module) (BSL 1.1). See [LICENSE](LICENSE) for details.
