# 生产级进阶：Module、DI 与生命周期

本文面向已掌握基础路由和中间件的开发者，介绍如何用 Astra 的 Module 系统和 DI 容器组织中大型项目。

> **前提**：先完成 [quickstart.md](quickstart.md) 的三步入门，再阅读本文。

---

## 概念层级（按需引入）

```
第一层（必须）：App + 路由 + 中间件
    ↓ 项目拆多文件时
第二层（推荐）：Module — 把路由/中间件/生命周期打包成可复用单元
    ↓ 需要第三方库集成时
第三层（可选）：Plugin — 横切关注点（Prometheus、Swagger、OAuth2 等）
    ↓ 依赖关系复杂时
第四层（可选）：di.Container — 跨模块依赖注入
```

大多数项目只需要第一层和第二层。

---

## 第二层：Module 系统

### 为什么需要 Module？

当项目超过一个文件时，路由注册代码会散落在 `main.go` 各处：

```go
// 没有 Module 时，main.go 越来越臃肿
func main() {
    app := astra.New()
    app.Use(...)

    // 用户路由
    userGroup := app.Group("/api/users")
    userGroup.GET("", listUsers)
    userGroup.POST("", createUser)
    // ... 10 个路由

    // 订单路由
    orderGroup := app.Group("/api/orders")
    orderGroup.GET("", listOrders)
    // ... 8 个路由

    // 初始化数据库
    db, _ := gorm.Open(...)
    app.OnStart(func(ctx context.Context) error {
        return db.AutoMigrate(...)
    })
    app.OnStop(func(ctx context.Context) error {
        sqlDB, _ := db.DB()
        return sqlDB.Close()
    })

    app.Run(":8080")
}
```

Module 让每个功能域自我管理：

```go
// 每个域一个 Module，main.go 只负责组装
func main() {
    db, _ := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})

    app := astra.New()
    app.Use(middleware.Logger(), middleware.Recovery(), middleware.CORS())

    app.Register(
        user.NewModule(db),
        order.NewModule(db),
        health.NewModule(db),
    )

    app.Run(":8080")
}
```

### Module 接口

```go
type Module interface {
    Name() string              // 唯一名称，用于日志和重复检测
    Install(app *App) error    // 安装入口：注册路由、中间件、生命周期钩子
}
```

### 实现一个 Module

```go
// internal/user/module.go
package user

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
    "gorm.io/gorm"
)

type Module struct {
    db *gorm.DB
}

func NewModule(db *gorm.DB) *Module {
    return &Module{db: db}
}

func (m *Module) Name() string { return "users" }

func (m *Module) Install(app *astra.App) error {
    h := newHandler(m.db)

    g := app.Group("/api/v1/users", middleware.JWT([]byte(os.Getenv("JWT_SECRET"))))
    g.GET("",      h.List)
    g.GET("/:id",  h.Get)
    g.POST("",     h.Create)
    g.PUT("/:id",  h.Update)
    g.DELETE("/:id", h.Delete)

    // 生命周期钩子（可选）
    app.OnStart(func(ctx context.Context) error {
        return m.db.AutoMigrate(&User{})
    })

    return nil
}
```

### ModuleFunc — 轻量内联模块

不需要完整 struct 时：

```go
app.Register(
    astra.NewModuleFunc("dev-tools", func(a *astra.App) error {
        if os.Getenv("APP_ENV") == "development" {
            a.Use(middleware.Pprof())
        }
        return nil
    }),
)
```

---

## 第三层：Plugin

Plugin 和 Module 的区别：

| | Module | Plugin |
|--|--------|--------|
| **适用场景** | 应用自身的业务单元 | 第三方库集成（Prometheus、Swagger 等） |
| **接口方法** | `Install(app *App) error` | `Init(app *App) error` |
| **注册方式** | `app.Register(m)` | `app.RegisterPlugin(p)` |

```go
// 使用内置 Plugin
app.RegisterPlugin(
    &swagger.Plugin{Path: "/docs"},
    &prometheus.Plugin{Path: "/metrics"},
)

// 或者混合注册
app.Register(
    astra.PluginAsModule(&swagger.Plugin{Path: "/docs"}),
    user.NewModule(db),
)
```

---

## 第四层：DI 容器

当多个 Module 共享同一个依赖（如数据库连接）时，DI 容器避免手动传参：

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/di"
    "github.com/astra-go/astra/middleware"
    "gorm.io/gorm"
)

func main() {
    c := di.New()

    // 注册依赖
    di.Provide[*gorm.DB](c, func() (*gorm.DB, error) {
        return gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})
    })

    // 绑定生命周期到 App
    app := astra.New()
    c.BindApp(app)

    // Module 从容器中取依赖
    app.Register(
        user.NewModuleWithDI(c),
        order.NewModuleWithDI(c),
    )

    app.Run(":8080")
}
```

```go
// Module 内部使用 DI
func NewModuleWithDI(c *di.Container) *Module {
    return &Module{
        db: di.MustInvoke[*gorm.DB](c),
    }
}
```

### 何时使用 DI？

| 场景 | 建议 |
|------|------|
| 1-3 个 Module，依赖简单 | 直接传参，不需要 DI |
| 5+ 个 Module，共享 DB/Redis/Config | 使用 DI 容器 |
| 需要懒初始化或条件依赖 | 使用 DI 容器 |

---

## 生命周期管理

Astra 提供 `OnStart` / `OnStop` 钩子，用于在服务启动/关闭时执行初始化/清理逻辑：

```go
app.OnStart(func(ctx context.Context) error {
    // 在 HTTP 服务器开始监听之前执行
    // 适合：数据库迁移、预热缓存、建立连接池
    return db.AutoMigrate(&User{}, &Order{})
})

app.OnStop(func(ctx context.Context) error {
    // 在 HTTP 服务器停止接受新请求之后执行
    // 适合：关闭数据库连接、刷新日志缓冲区
    sqlDB, _ := db.DB()
    return sqlDB.Close()
})
```

收到 `SIGINT` / `SIGTERM` 时，Astra 自动执行优雅关闭：
1. 停止接受新连接
2. 等待进行中的请求完成（默认超时 30 秒）
3. 依次执行 `OnStop` 钩子

---

## 完整示例：中型服务

```go
// cmd/server/main.go
package main

import (
    "context"
    "os"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/di"
    "github.com/astra-go/astra/middleware"
    "github.com/astra-go/astra/orm"

    "myapp/internal/user"
    "myapp/internal/order"
)

func main() {
    // 依赖容器
    c := di.New()
    di.Provide[*gorm.DB](c, func() (*gorm.DB, error) {
        return orm.Open(orm.PostgreSQL(os.Getenv("DATABASE_URL")))
    })

    // 应用
    app := astra.New(
        astra.WithShutdownTimeout(30),
    )
    c.BindApp(app)

    // 全局中间件
    app.Use(
        middleware.RequestID(),
        middleware.Logger(),
        middleware.Recovery(),
        middleware.CORS(),
        middleware.SecureHeaders(),
    )

    // 注册模块
    if err := app.Register(
        user.NewModule(c),
        order.NewModule(c),
        health.NewModule(c),
    ); err != nil {
        slog.Error("module registration failed", "err", err)
        os.Exit(1)
    }

    slog.Info("starting server", "addr", ":8080")
    if err := app.Run(":8080"); err != nil {
        slog.Error("server error", "err", err)
        os.Exit(1)
    }
}
```

---

## 推荐目录结构

```
myapp/
├── cmd/
│   └── server/
│       └── main.go          # 入口：组装依赖、注册模块、启动
├── internal/
│   ├── user/
│   │   ├── module.go        # Module 实现
│   │   ├── handler.go       # HTTP handler
│   │   ├── service.go       # 业务逻辑
│   │   ├── repository.go    # 数据访问
│   │   └── model.go         # 数据模型
│   ├── order/
│   │   └── ...
│   └── middleware/          # 项目自定义中间件
├── config/
│   └── config.yaml
├── migrations/
├── go.mod
└── go.sum
```

使用 `astractl` 快速生成骨架：

```bash
astractl new myapp --layout ddd
astractl gen crud User --with-service
```
