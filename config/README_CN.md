# Config — 多源配置管理

支持从文件（YAML/JSON/TOML）、环境变量、内存和远程配置中心（etcd/Consul/Nacos/Apollo/Vault）加载配置，支持热更新。

## 特性

- **多源合并**：多个配置源从左到右加载；后加载的源覆盖先加载的
- **环境变量映射**：支持 `APP__DB__PORT=5432` → `db.port=5432` 嵌套映射
- **结构体绑定**：`Scan` 方法自动将配置映射到 Go 结构体，支持 `default` tag
- **热更新**：文件源通过 fsnotify 监控变化；远程源支持 Watch 推送
- **Tag 支持**：`default`、`required`、`env` 等结构体 tag

## 快速开始

```go
cfg := config.New(
    config.YAMLFile{Path: "config.yaml"},
    config.JSONFile{Path: "config.json"},
    config.Env{Prefix: "APP"},
    config.Memory{Data: map[string]any{"debug": true}},
)

type AppConfig struct {
    Port    int           `yaml:"port"    default:"8080"`
    Debug   bool          `yaml:"debug"   default:"false"`
    Timeout time.Duration `yaml:"timeout" default:"30s"`
}

var appCfg AppConfig
cfg.Scan(&appCfg)
fmt.Println(appCfg.Port) // 输出配置的端口
```

## 热更新

```go
// 方式一：自动监控文件变化并重新 Scan
cfg.StartWatch(ctx)

// 方式二：注册回调手动处理变化
cfg.Watch(func() {
    var cfg AppConfig
    cfg.Scan(&cfg)
    applyConfig(cfg)
})
```

## API

### config.New

```go
func New(sources ...config.Source) *Config
```

`Source` 支持以下类型：

| 类型 | 说明 |
|------|-------------|
| `YAMLFile{Path}` | YAML 配置文件 |
| `JSONFile{Path}` | JSON 配置文件 |
| `TOMLFile{Path}` | TOML 配置文件 |
| `Env{Prefix}` | 环境变量，`Prefix__KEY__SUB=value` 映射到嵌套结构体 |
| `Memory{Data}` | 直接传入 `map[string]any` |
| `etcd.Source` | etcd v3 配置中心 |
| `consul.Source` | Consul KV 配置中心 |
| `nacos.Source` | Nacos 配置中心 |
| `apollo.Source` | Apollo 配置中心 |
| `vault.Source` | HashiCorp Vault |

### cfg.Scan

```go
func (c *Config) Scan(v any) error
```

将当前配置值绑定到 `v`（必须为指针）。支持 `default` tag 设置默认值。

### cfg.StartWatch

```go
func (c *Config) StartWatch(ctx context.Context)
```

启动文件监控（基于 fsnotify）。所有文件源变化自动重载。

### cfg.Watch

```go
func (c *Config) Watch(fn func())
```

注册变更回调函数。

### 远程配置源

```go
// etcd
import "github.com/astra-go/astra/config/etcd"
src := etcd.NewClient(etcdConfig)
cfg := config.New(src)

// Consul
import "github.com/astra-go/astra/config/consul"
src := consul.NewClient(consulConfig)
cfg := config.New(src)

// Nacos
import "github.com/astra-go/astra/config/nacos"
src, _ := nacos.NewSource(nacos.Config{...})
cfg := config.New(src)

// Apollo
import "github.com/astra-go/astra/config/apollo"
src, _ := apollo.NewSource(apollo.Config{...})
cfg := config.New(src)

// Vault
import "github.com/astra-go/astra/config/vault"
src, _ := vault.NewSource(vault.Config{...})
cfg := config.New(src)
```

## 配置 Tag

| Tag | 示例 | 说明 |
|-----|---------|-------------|
| `default:"8080"` | `Port int default:"8080"` | 未配置时的默认值 |
| `required:"true"` | `Token string required:"true"` | 缺失时报错 |
| `env:"APP_TOKEN"` | `Token string env:"APP_TOKEN"` | 从环境变量读取 |

### 环境变量映射规则

前缀为 `APP`，嵌套 key 用双下划线 `__` 分隔：

```bash
APP__DB__HOST=localhost
APP__DB__PORT=5432
```

对应结构体：

```go
type DBConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"

    "github.com/astra-go/astra/config"
)

type AppConfig struct {
    Port    int      `yaml:"port"    default:"8080"`
    Debug   bool     `yaml:"debug"   default:"false"`
    DB      DBConfig `yaml:"db"`
}

type DBConfig struct {
    Host string `yaml:"host" default:"localhost"`
    Port int    `yaml:"port" default:"5432"`
}

func main() {
    cfg := config.New(
        config.YAMLFile{Path: "config.yaml"},
        config.Env{Prefix: "APP"},
    )

    var app AppConfig
    if err := cfg.Scan(&app); err != nil {
        panic(err)
    }
    fmt.Printf("Port: %d, DB: %s:%d\n", app.Port, app.DB.Host, app.DB.Port)

    // 热更新
    ctx := context.Background()
    cfg.Watch(func() {
        var a AppConfig
        cfg.Scan(&a)
        fmt.Println("Config reloaded:", a.Port)
    })
    cfg.StartWatch(ctx)
}
```

## 模块依赖

| 子包 | 依赖 |
|-------------|-----------|
| `config/etcd` | `go.etcd.io/etcd/client/v3` |
| `config/consul` | `github.com/hashicorp/consul/api` |
| `config/nacos` | `github.com/nacos-group/nacos-sdk-go` |
| `config/apollo` | `github.com/ZhangJZ99/apollo-client` |
| `config/vault` | `github.com/hashicorp/vault/api` |

## 注意事项

- 配置 Scan 不会自动热更新结构体；使用 `Watch` 回调手动处理变化
- 环境变量前缀推荐大写
- 远程配置源（etcd/Consul 等）实现 `Watchable` 接口，支持推送更新