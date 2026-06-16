# Discovery — 服务注册与发现

服务注册与发现抽象，支持多种注册中心（Consul、etcd、Kubernetes、Nacos）。

## 特性

- **统一抽象**：Registrar 和 Resolver 均为插件化
- **实例元数据**：支持标签、权重、健康检查信息
- **内存注册中心**：`NewInMemoryRegistry` 适用于本地开发和小型部署
- **Kubernetes Resolver**：通过 API Server 发现 Pod 端点
- **Nacos**：同时支持服务注册和发现

## 快速开始

### 内存注册中心（开发/测试）

```go
reg := discovery.NewInMemoryRegistry()
reg.Register(ctx, &discovery.ServiceInstance{
    Name:    "user-svc",
    Address: "10.0.0.1",
    Port:    8080,
    Weight:  100,
})
```

### Consul 注册与发现

```go
// 注册服务
registrar := discovery.NewConsulRegistrar(consul.Config{
    Address: "localhost:8500",
    Service: "user-svc",
    Address: "10.0.0.1",
    Port:    8080,
})
registrar.Register(ctx)

// 发现服务
resolver := discovery.NewConsulResolver(consul.Config{Address: "localhost:8500"})
instances, _ := resolver.Resolve(ctx, "user-svc")
```

### etcd 注册与发现

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

### Nacos 注册与发现

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

### ServiceInstance 结构体

```go
type ServiceInstance struct {
    Name      string            // 服务名（必须全局唯一）
    Address   string            // IP 或域名
    Port      int               // 端口
    Weight    int               // 权重（用于加权负载均衡）
    Enable    bool              // 是否启用
    Healthy   bool              // 健康状态
    Metadata  map[string]string // 额外元数据（如 version、region）
}
```

### Registrar 接口

```go
type Registrar interface {
    Register(ctx context.Context, instance *ServiceInstance) error
    Deregister(ctx context.Context, instanceID string) error
}
```

### Resolver 接口

```go
type Resolver interface {
    Resolve(ctx context.Context, name string) ([]*ServiceInstance, error)
}
```

### Registry 接口（内存）

```go
type Registry interface {
    Registrar
    Resolver
    // 服务实例管理
    ListServices(ctx context.Context) ([]*ServiceInstance, error)
}
```

## 配置

### ConsulConfig

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Address` | `string` | — | Consul Agent 地址 |
| `Service` | `string` | — | 服务名 |
| `Address` | `string` | — | 实例地址 |
| `Port` | `int` | — | 实例端口 |
| `Check` | `*CheckConfig` | — | 健康检查配置 |

### NacosConfig

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Address` | `string` | — | Nacos Server 地址 |
| `Namespace` | `string` | `public` | Namespace |
| `Group` | `string` | `DEFAULT_GROUP` | Group |

## 模块依赖

| 子包 | 依赖 |
|-------------|-----------|
| `discovery/consul` | `github.com/hashicorp/consul/api` |
| `discovery/etcd` | `go.etcd.io/etcd/client/v3` |
| `discovery/nacos` | `github.com/nacos-group/nacos-sdk-go` |

## 注意事项

- 生产环境推荐使用 Consul/etcd 实现高可用注册中心
- 服务注销时主动调用 `Deregister`，避免心跳过期延迟导致请求打到已下线实例
- `Metadata` 可存储服务版本和区域信息，实现更精细的路由策略