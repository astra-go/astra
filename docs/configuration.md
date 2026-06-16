# Configuration

Astra provides a flexible multi-source configuration management system supporting local files, environment variables, remote config centers, and hot reload.

## Quick Start

```go
package main

import (
    "log"
    "github.com/astra-go/astra/config"
)

type AppConfig struct {
    Port    int      `yaml:"port"    default:"8080"`
    Debug   bool     `yaml:"debug"   default:"false"`
    DB      DBConfig `yaml:"db"`
}

type DBConfig struct {
    Host     string `yaml:"host"     default:"localhost"`
    Port     int    `yaml:"port"     default:"5432"`
    Database string `yaml:"database" default:"myapp"`
    User     string `yaml:"user"`
    Password string `yaml:"password"`
}

func main() {
    // Create config manager
    cfg := config.New(
        config.YAMLFile{Path: "config.yaml"},       // Load YAML first
        config.Env{Prefix: "APP"},                  // Env vars override (APP__DB__HOST=...)
    )

    // Bind to struct (auto-fills default tags)
    var appCfg AppConfig
    if err := cfg.Scan(&appCfg); err != nil {
        log.Fatal(err)
    }

    log.Printf("Starting on :%d", appCfg.Port)
}
```

## Config Sources

### File Sources

Supports YAML, JSON, and TOML:

```go
cfg := config.New(
    config.YAMLFile{Path: "/etc/myapp/config.yaml"},
    config.JSONFile{Path: "config.json"},
    config.TOMLFile{Path: "config.toml"},
)
```

Example `config.yaml`:

```yaml
server:
  port: 8080
  debug: false

db:
  host: localhost
  port: 5432
  database: myapp
  user: admin
  password: secret

redis:
  addr: localhost:6379
  db: 0
```

### Environment Variables

```go
// Prefix APP, separator __
// APP__SERVER__PORT=9090 → server.port=9090
// APP__DB__HOST=prod-db  → db.host=prod-db
cfg.Use(config.Env{Prefix: "APP"})
```

### Memory Source

```go
cfg.Use(config.Memory{Data: map[string]any{
    "debug": true,
    "port": 9090,
}})
```

### Remote Config Sources

```go
import (
    "github.com/astra-go/astra/config/etcd"
    "github.com/astra-go/astra/config/nacos"
    "github.com/astra-go/astra/config/apollo"
)

// etcd
etcdSrc, _ := etcd.NewSource(etcd.Config{
    Endpoints: []string{"localhost:2379"},
    Key:       "/config/myapp",
})

// Nacos
nacosSrc, _ := nacos.NewSource(nacos.Config{
    Endpoint:  "localhost:8848",
    Namespace: "public",
    DataID:    "myapp.yaml",
    Group:     "DEFAULT_GROUP",
})

// Apollo
apolloSrc, _ := apollo.NewSource(apollo.Config{
    AppID:   "myapp",
    Cluster: "default",
    Namespaces: []string{"application"},
    MetaAddr: "http://localhost:8080",
})

cfg.Use(etcdSrc, nacosSrc, apolloSrc)
```

## Hot Reload

```go
cfg := config.New(config.YAMLFile{Path: "config.yaml"})

// Start watching for file changes
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go func() {
    if err := cfg.StartWatch(ctx); err != nil {
        log.Fatal(err)
    }
}()

// Register change callback
cfg.Watch(func() {
    log.Println("Config updated, reloading...")
    var appCfg AppConfig
    cfg.Scan(&appCfg)
    // Dynamically adjust service
})
```

## Reading Config

```go
// Read value directly
port := cfg.Get("server.port").Int()     // int
debug := cfg.Get("server.debug").Bool()   // bool
host := cfg.Get("db.host").String()       // string
timeout := cfg.Get("server.timeout").Duration() // time.Duration

// Safe read with default
port := cfg.Get("server.port").DefaultInt(8080)

// Full config snapshot
all := cfg.All() // map[string]any

// Subset extraction
dbConfig := cfg.Sub("db") // Returns sub-config view of db.xxx
```

## Config Validation

```go
type Config struct {
    Port int    `yaml:"port" validate:"required,min=80,max=65535"`
    Host string `yaml:"host" validate:"required,hostname"`
}

var cfg Config
if err := config.Scan(&cfg); err != nil {
    log.Fatal(err)
}
if err := config.Validate(&cfg); err != nil {
    log.Fatalf("Config validation failed: %v", err)
}
```

## Integration with Astra App

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/config"
)

func main() {
    // Load config
    cfg := config.New(
        config.YAMLFile{Path: "app.yaml"},
        config.Env{Prefix: "APP"},
    )

    var cfg AppConfig
    cfg.Scan(&appCfg)

    // Create App
    app := astra.New(
        astra.WithMode(map[bool]astra.Mode{true: astra.ModeProd, false: astra.ModeDev}[appCfg.Debug]),
    )

    // Store config for later use
    app.OnStart(func(ctx context.Context) error {
        app.Set("config", appCfg)
        return nil
    })

    app.Run(fmt.Sprintf(":%d", appCfg.Port))
}
```

## Best Practices

1. **Layered Config**: defaults → file → env → remote (later overrides earlier)
2. **Sensitive Info**: passwords and keys injected via env vars or Vault, never committed to repo
3. **Environment Separation**: use `APP_ENV=production` to load different config files
4. **Hot Reload Safety**: don't replace objects in use directly in change callbacks; use atomic pointers
5. **Config Validation**: validate all config at startup, avoid runtime surprises
