# 推荐项目结构

```
myapp/
├── cmd/
│   └── server/
│       └── main.go          # 入口：App 初始化、依赖注入、启动
├── internal/
│   ├── handler/             # HTTP handler 函数
│   │   ├── user.go
│   │   └── order.go
│   ├── service/             # 业务逻辑（无框架依赖）
│   ├── repository/          # 数据访问层
│   └── middleware/          # 项目自定义中间件
├── config/
│   └── config.yaml          # 配置文件
├── docs/                    # API 文档（Swagger/OpenAPI）
├── migrations/              # 数据库迁移文件
├── go.mod
└── go.sum
```

## 入口示例

```go
// cmd/server/main.go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
    "myapp/internal/handler"
)

func main() {
    app := astra.New(
        astra.WithLogger(initLogger()),
        astra.WithTrustedProxies("10.0.0.0/8"),
    )

    app.Use(
        middleware.Recovery(),
        middleware.Logger(),
        middleware.RequestID(),
        middleware.SecureHeaders(),
    )

    handler.RegisterRoutes(app)

    app.OnStart(initDB)
    app.OnStop(closeDB)

    app.Run(":8080")
}
```

## Handler 文件示例

```go
// internal/handler/user.go
package handler

import "github.com/astra-go/astra"

func RegisterRoutes(app *astra.App) {
    g := app.Group("/api/v1/users")
    g.GET("",     listUsers)
    g.POST("",    createUser)
    g.GET("/:id", getUser)
}
```
