# Discovery — Service Registration and Discovery

Service registration and discovery abstraction supporting multiple registries (Consul, etcd, Kubernetes, Nacos).

## Features

- **Unified Abstraction**: Registrar and Resolver are both plugin-based
- **Instance Metadata**: Supports tags, weight, health check info
- **In-Memory Registry**: `NewInMemoryRegistry` suitable for local development and small-scale deployments
- **Kubernetes Resolver**: Discovers Pod endpoints via API Server
- **Nacos**: Supports both service registration and discovery

## Quick Start

### In-Memory Registry (Dev/Testing)

```go
reg := discovery.NewInMemoryRegistry()
reg.Register(ctx, &discovery.ServiceInstance{
    Name:    "user-svc",
    Address: "10.0.0.1",
    Port:    8080,
    Weight:  100,
})
```

### Consul Registration and Discovery

```go
// Register service
registrar := discovery.NewConsulRegistrar(consul.Config{
    Address: "localhost:8500",
    Service: "user-svc",
    Address: "10.0.0.1",
    Port:    8080,
})
registrar.Register(ctx)

// Discover service
resolver := discovery.NewConsulResolver(consul.Config{Address: "localhost:8500"})
instances, _ := resolver.Resolve(ctx, "user-svc")
```

### etcd Registration and Discovery

```go
registrar := discovery.NewEtcdRegistrar(etcd.Config{
    Addr: []string{"localhost:2379"},
})
registrar.Register(ctx)

resolver := discovery.NewEtcdResolver(etcd.Config{
    Addr: []string{"localhost:2379"},
})
instances, _ := resolver.Resolve(ctx, "user-svc")
```

### Nacos Registration and Discovery

```go
registrar := discovery.NewNacosRegistrar(nacos.Config{
    Address: "localhost:8848",
    Namespace: "public",
})
registrar.Register(ctx)

resolver := discovery.NewNacosResolver(nacos.Config{
    Address: "localhost:8848",
})
instances, _ := resolver.Resolve(ctx, "user-svc")
```

## API

### ServiceInstance Structure

```go
type ServiceInstance struct {
    Name      string            // Service name (must be globally unique)
    Address   string            // IP or domain
    Port      int               // Port
    Weight    int               // Weight (for weighted load balancing)
    Enable    bool              // Whether enabled
    Healthy   bool              // Health status
    Metadata  map[string]string // Additional metadata (e.g., version, region)
}
```

### Registrar Interface

```go
type Registrar interface {
    Register(ctx context.Context, instance *ServiceInstance) error
    Deregister(ctx context.Context, instanceID string) error
}
```

### Resolver Interface

```go
type Resolver interface {
    Resolve(ctx context.Context, name string) ([]*ServiceInstance, error)
}
```

### Registry Interface (In-Memory)

```go
type Registry interface {
    Registrar
    Resolver
    // Service instance management
    ListServices(ctx context.Context) ([]*ServiceInstance, error)
}
```

## Config

### ConsulConfig

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Address` | `string` | — | Consul Agent address |
| `Service` | `string` | — | Service name |
| `Address` | `string` | — | Instance address |
| `Port` | `int` | — | Instance port |
| `Check` | `*CheckConfig` | — | Health check config |

### NacosConfig

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Address` | `string` | — | Nacos Server address |
| `Namespace` | `string` | `public` | Namespace |
| `Group` | `string` | `DEFAULT_GROUP` | Group |

## Module Dependencies

| Sub-package | Dependency |
|-------------|-----------|
| `discovery/consul` | `github.com/hashicorp/consul/api` |
| `discovery/etcd` | `go.etcd.io/etcd/client/v3` |
| `discovery/nacos` | `github.com/nacos-group/nacos-sdk-go` |

## Notes

- Production recommends Consul/etcd for high availability registries
- On service deregistration, call `Deregister` to actively unregister to avoid heartbeat expiry delay causing requests to hit already-down instances
- `Metadata` can store service version and region info, enabling more granular routing strategies
