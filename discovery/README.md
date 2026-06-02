# Discovery Module

服务发现模块，支持多种服务注册中心后端。

## 特性

- 🎯 **统一接口**：所有后端实现相同的 `Registry` 接口
- 🔌 **可插拔后端**：支持 Consul、etcd、Kubernetes、Nacos
- 🏷️ **按需编译**：使用 build tags 控制编译哪些后端
- 🔄 **实时监听**：通过 `Watch` 方法实时获取服务变更
- 🔒 **并发安全**：所有实现都是线程安全的

## 支持的后端

| 后端       | Build Tag | 特点                          |
|-----------|-----------|-------------------------------|
| Consul    | `consul`  | 成熟稳定，支持健康检查、KV 存储 |
| etcd      | `etcd`    | 强一致性，自动过期（TTL）      |
| Kubernetes| `k8s`     | 云原生，与 K8s 生态深度集成    |
| Nacos     | `nacos`   | 阿里开源，支持配置管理         |

## 快速开始

### 安装

```bash
go get github.com/astra-go/astra/discovery@v2.0.0
```

### 基本用法

所有后端都实现了相同的接口：

```go
type Registry interface {
    Register(ctx context.Context, instance *ServiceInstance) error
    Deregister(ctx context.Context, instanceID string) error
    Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error)
    Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error)
    Close() error
}
```

### Consul 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/discovery"
    "github.com/hashicorp/consul/api"
)

func main() {
    // 创建 Consul 客户端
    cfg := api.DefaultConfig()
    cfg.Address = "localhost:8500"
    
    // 创建注册中心
    reg, err := discovery.NewConsulRegistryFromConfig(cfg)
    if err != nil {
        panic(err)
    }
    defer reg.Close()
    
    ctx := context.Background()
    
    // 注册服务实例
    err = reg.Register(ctx, &discovery.ServiceInstance{
        ID:      "user-svc-1",
        Name:    "user-svc",
        Address: "10.0.0.1:8080",
        Scheme:  "http",
        Weight:  1,
        Metadata: map[string]string{
            "version": "v1.0.0",
            "region":  "us-west-1",
        },
    })
    
    // 发现服务
    instances, err := reg.Discover(ctx, "user-svc")
    for _, inst := range instances {
        fmt.Printf("Found: %s at %s\n", inst.ID, inst.Address)
    }
    
    // 监听服务变更
    ch, err := reg.Watch(ctx, "user-svc")
    for instances := range ch {
        fmt.Printf("Service updated: %d instances\n", len(instances))
    }
}
```

### etcd 示例

```go
package main

import (
    "context"
    "time"
    "github.com/astra-go/astra/discovery"
    clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
    // 创建 etcd 客户端
    cli, err := clientv3.New(clientv3.Config{
        Endpoints:   []string{"localhost:2379"},
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        panic(err)
    }
    
    // 创建注册中心（使用 /services 作为前缀）
    reg := discovery.NewEtcdRegistry(cli, "/services")
    defer reg.Close()
    
    ctx := context.Background()
    
    // 注册服务（自动续约）
    err = reg.Register(ctx, &discovery.ServiceInstance{
        ID:      "api-svc-1",
        Name:    "api-svc",
        Address: "192.168.1.10:8080",
    })
}
```

### Kubernetes 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/discovery"
)

func main() {
    // In-cluster 模式（Pod 内运行）
    reg, err := discovery.NewK8sRegistry(discovery.K8sConfig{
        Namespace: "production",
        InCluster: true,
    })
    if err != nil {
        panic(err)
    }
    defer reg.Close()
    
    ctx := context.Background()
    
    // 发现服务（通过 Endpoints）
    instances, err := reg.Discover(ctx, "my-service")
    for _, inst := range instances {
        fmt.Printf("Pod: %s at %s\n", inst.ID, inst.Address)
    }
}
```

### Nacos 示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/discovery"
    "github.com/nacos-group/nacos-sdk-go/v2/clients"
    "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
    "github.com/nacos-group/nacos-sdk-go/v2/vo"
)

func main() {
    // 创建 Nacos 客户端
    sc := []constant.ServerConfig{{
        IpAddr: "127.0.0.1",
        Port:   8848,
    }}
    cc := constant.NewClientConfig(
        constant.WithNamespaceId("public"),
        constant.WithTimeoutMs(5000),
    )
    
    namingClient, err := clients.NewNamingClient(vo.NacosClientParam{
        ClientConfig:  cc,
        ServerConfigs: sc,
    })
    if err != nil {
        panic(err)
    }
    
    // 创建注册中心
    reg := discovery.NewNacosRegistry(namingClient)
    defer reg.Close()
    
    ctx := context.Background()
    
    // 注册服务
    err = reg.Register(ctx, &discovery.ServiceInstance{
        ID:      "order-svc-1",
        Name:    "order-svc",
        Address: "172.16.0.10:8080",
    })
}
```

## 编译标签

使用 build tags 控制编译哪些后端，减少二进制体积：

```bash
# 编译所有后端
go build -tags=alltags

# 仅编译 Consul
go build -tags=consul

# 编译多个后端
go build -tags="consul,etcd,k8s"
```

## 测试

```bash
# 测试所有后端
go test -tags=alltags ./...

# 测试特定后端
go test -tags=consul ./...
```

## ServiceInstance 结构

```go
type ServiceInstance struct {
    ID       string            // 实例唯一标识
    Name     string            // 服务名称
    Address  string            // 地址（host:port）
    Scheme   string            // 协议（http/https/grpc）
    Weight   int               // 负载均衡权重
    Metadata map[string]string // 自定义元数据
}
```

## 错误处理

```go
import "errors"

// 服务未找到
if errors.Is(err, discovery.ErrNotFound) {
    // 处理服务不存在的情况
}

// 实例 ID 为空
if errors.Is(err, discovery.ErrInstanceIDEmpty) {
    // 处理 ID 校验失败
}
```

## 最佳实践

### 1. 使用 Context 控制超时

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

instances, err := reg.Discover(ctx, "my-service")
```

### 2. 优雅关闭

```go
defer reg.Close()

// 注销服务
if err := reg.Deregister(ctx, instanceID); err != nil {
    log.Printf("Failed to deregister: %v", err)
}
```

### 3. Watch 监听服务变更

```go
ch, err := reg.Watch(ctx, "my-service")
if err != nil {
    return err
}

for {
    select {
    case instances := <-ch:
        // 更新本地缓存
        updateCache(instances)
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 4. 健康检查与自动续约

大多数后端会自动处理心跳/续约：

- **Consul**: 使用 TTL 健康检查，后台自动续约
- **etcd**: 使用 Lease + KeepAlive 机制
- **Nacos**: 支持临时实例（Ephemeral），自动心跳
- **Kubernetes**: 依赖 Endpoints 控制器

### 5. 元数据的合理使用

```go
instance := &discovery.ServiceInstance{
    ID:      "svc-1",
    Name:    "user-svc",
    Address: "10.0.0.1:8080",
    Metadata: map[string]string{
        "version":     "v1.2.3",
        "datacenter":  "us-west-1",
        "environment": "production",
        "protocol":    "grpc",
    },
}
```

## 性能考虑

### 二进制体积

使用 build tags 可以显著减少二进制体积：

| 后端组合        | 增加体积（大致）|
|----------------|----------------|
| Consul only    | +5.2 MB        |
| etcd only      | +7.1 MB        |
| K8s only       | +10.3 MB       |
| Nacos only     | +4.1 MB        |
| All backends   | +22.7 MB       |

### 并发性能

所有实现都是线程安全的，支持高并发调用：

```go
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        reg.Register(ctx, &discovery.ServiceInstance{
            ID:   fmt.Sprintf("inst-%d", id),
            Name: "svc",
        })
    }(i)
}
wg.Wait()
```

## 迁移指南

如果你从 v1.x 迁移到 v2.x，请参考 [迁移指南](../docs/migration-guide-discovery-v2.md)。

## 相关文档

- [ADR-005: 子模块数量上限策略](../docs/adr/ADR-005-module-count-limit.md)
- [P1-3 完整技术分析](../docs/analysis-p1-3-discovery-consolidation.md)
- [API 文档](https://pkg.go.dev/github.com/astra-go/astra/discovery)

## 许可证

MIT
