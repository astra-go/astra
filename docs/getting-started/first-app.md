# 第一个应用

## 创建项目

```bash
mkdir myapp && cd myapp
go mod init myapp
go get github.com/astra-go/astra@latest
```

## Hello World

```go
// main.go
package main

import (
    "log/slog"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

func main() {
    app := astra.New()

    app.Use(
        middleware.Recovery(),
        middleware.Logger(),
        middleware.RequestID(),
    )

    app.GET("/ping", func(c *astra.Context) error {
        return c.JSON(200, astra.H{"message": "pong"})
    })

    app.GET("/hello/:name", func(c *astra.Context) error {
        name := c.Param("name")
        return c.String(200, "Hello, %s!", name)
    })

    slog.Info("starting", "addr", ":8080")
    if err := app.Run(":8080"); err != nil {
        slog.Error("startup failed", "err", err)
    }
}
```

```bash
go run main.go

# 另一个终端
curl http://localhost:8080/ping
# {"message":"pong"}

curl http://localhost:8080/hello/Astra
# Hello, Astra!
```

## 添加路由组和中间件

```go
api := app.Group("/api/v1")

// 公开路由
api.POST("/auth/login", loginHandler)

// 需要 JWT 认证的路由
protected := api.Group("",
    middleware.JWT(middleware.JWTConfig{SigningKey: []byte("secret")}),
)
protected.GET("/profile", getProfile)
protected.PUT("/profile", updateProfile)
```

## 请求绑定与校验

```go
type CreateUserReq struct {
    Name  string `json:"name"  validate:"required,max=64"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age"   validate:"gte=0,lte=150"`
}

app.POST("/users", func(c *astra.Context) error {
    var req CreateUserReq
    if err := c.ShouldBind(&req); err != nil {
        return err  // 框架自动渲染 422 + 字段错误详情
    }
    user, err := userSvc.Create(c.Request().Context(), req)
    if err != nil {
        return astra.NewHTTPError(500, "create failed")
    }
    return c.JSON(201, user)
})
```

## 优雅关闭

框架自动处理 `SIGINT` / `SIGTERM`。可添加自定义清理逻辑：

```go
app.OnStart(func(ctx context.Context) error {
    return db.Connect(ctx)
})
app.OnStop(func(ctx context.Context) error {
    return db.Close(ctx)
})
app.Run(":8080")
```
