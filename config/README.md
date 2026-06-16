# Config — Multi-Source Configuration Management

Supports loading config from files (YAML/JSON/TOML), environment variables, memory, and remote config centers (etcd/Consul/Nacos/Apollo/Vault), with hot reload support.

## Features

- **Multi-Source Merge**: Multiple config sources load left-to-right; later sources override earlier ones
- **Environment Variable Mapping**: Supports `APP__DB__PORT=5432` → `db.port=5432` nested mapping
- **Struct Binding**: `Scan` method auto-maps config to Go structs, supports `default` tags
- **Hot Reload**: File sources monitor changes via fsnotify; remote sources support Watch push
- **Tag Support**: `default`, `required`, `env` and other struct tags

## Quick Start

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
fmt.Println(appCfg.Port) // Outputs configured port
```

## Hot Reload

```go
// Method 1: Auto-monitor file changes and re-Scan
cfg.StartWatch(ctx)

// Method 2: Register callback for manual change handling
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

`Source` supports the following types:

| Type | Description |
|------|-------------|
| `YAMLFile{Path}` | YAML config file |
| `JSONFile{Path}` | JSON config file |
| `TOMLFile{Path}` | TOML config file |
| `Env{Prefix}` | Environment variables, `Prefix__KEY__SUB=value` maps to nested struct |
| `Memory{Data}` | Directly pass `map[string]any` |
| `etcd.Source` | etcd v3 config center |
| `consul.Source` | Consul KV config center |
| `nacos.Source` | Nacos config center |
| `apollo.Source` | Apollo config center |
| `vault.Source` | HashiCorp Vault |

### cfg.Scan

```go
func (c *Config) Scan(v any) error
```

Binds current config values to `v` (must be pointer). Supports `default` tag for default values.

### cfg.StartWatch

```go
func (c *Config) StartWatch(ctx context.Context)
```

Starts file monitoring (based on fsnotify). All file source changes auto-reload.

### cfg.Watch

```go
func (c *Config) Watch(fn func())
```

Registers a change callback function.

### Remote Config Sources

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

## Config Tags

| Tag | Example | Description |
|-----|---------|-------------|
| `default:"8080"` | `Port int default:"8080"` | Default value when not configured |
| `required:"true"` | `Token string required:"true"` | Error if missing |
| `env:"APP_TOKEN"` | `Token string env:"APP_TOKEN"` | Read from env var |

### Environment Variable Mapping Rules

Prefix `APP`, nested keys separated by double underscore `__`:

```bash
APP__DB__HOST=localhost
APP__DB__PORT=5432
```

Corresponds to struct:

```go
type DBConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}
```

## Complete Example

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

    // Hot reload
    ctx := context.Background()
    cfg.Watch(func() {
        var a AppConfig
        cfg.Scan(&a)
        fmt.Println("Config reloaded:", a.Port)
    })
    cfg.StartWatch(ctx)
}
```

## Module Dependencies

| Sub-package | Dependency |
|-------------|-----------|
| `config/etcd` | `go.etcd.io/etcd/client/v3` |
| `config/consul` | `github.com/hashicorp/consul/api` |
| `config/nacos` | `github.com/nacos-group/nacos-sdk-go` |
| `config/apollo` | `github.com/ZhangJZ99/apollo-client` |
| `config/vault` | `github.com/hashicorp/vault/api` |

## Notes

- Config Scan doesn't auto hot-reload structs; use `Watch` callback to handle changes manually
- Environment variable prefixes recommended to be uppercase
- Remote config sources (etcd/Consul, etc.) implement `Watchable` interface and support push updates
