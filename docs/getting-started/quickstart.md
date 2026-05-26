# Getting Started

Astra 的学习路径分三步，按需深入——无需一次掌握所有概念。

---

## Step 1 — Hello World（5 分钟）

和 Gin / Echo 写法完全一致，无任何额外概念。

```bash
mkdir myapp && cd myapp
go mod init myapp
go get github.com/astra-go/astra@latest
```

```go
// main.go
package main

import (
    "net/http"
    "github.com/astra-go/astra"
)

func main() {
    app := astra.New()

    app.GET("/", func(c astra.Context) error {
        return c.JSON(http.StatusOK, astra.Map{"message": "hello astra"})
    })

    app.GET("/hello/:name", func(c astra.Context) error {
        return c.JSON(http.StatusOK, astra.Map{"hello": c.Param("name")})
    })

    app.Run(":8080")
}
```

```bash
go run main.go
curl http://localhost:8080/hello/world
# {"hello":"world"}
```

完整示例：[`examples/hello`](../../examples/hello/main.go)

---

## Step 2 — 真实服务（15 分钟）

加入中间件、路由组、请求绑定与校验、优雅关闭。

```go
package main

import (
    "context"
    "fmt"
    "net/http"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/middleware"
)

type CreateItemReq struct {
    Name  string `json:"name"  validate:"required,max=64"`
    Price int    `json:"price" validate:"gte=0"`
}

func main() {
    app := astra.New()

    // 全局中间件
    app.Use(
        middleware.Recovery(),
        middleware.Logger(),
        middleware.RequestID(),
    )

    // 路由组
    v1 := app.Group("/api/v1")
    v1.GET("/items", listItems)
    v1.POST("/items", createItem)

    // 需要认证的子组
    admin := v1.Group("/admin")
    admin.Use(middleware.JWT("your-secret"))
    admin.GET("/stats", statsHandler)

    // 生命周期钩子（可选）
    app.OnStart(func(_ context.Context) error {
        fmt.Println("server ready")
        return nil
    })

    app.Run(":8080")
}
```

绑定与校验：框架在 `ShouldBind` 失败时自动返回 `422 + 字段错误`，无需手动处理。

```go
func createItem(c astra.Context) error {
    var req CreateItemReq
    if err := c.ShouldBind(&req); err != nil {
        return err // → 422 {"errors": [{"field":"name","msg":"required"}]}
    }
    // ...
    return c.JSON(http.StatusCreated, result)
}
```

完整示例：[`examples/quickstart`](../../examples/quickstart/main.go)

---

## Step 3 — 大型应用（按需）

当代码超过一个文件时，用 **Module** 拆分领域边界。

```go
// 每个 Module 独立注册路由、中间件和生命周期
type UserModule struct{ db *sql.DB }

func (m *UserModule) Name() string { return "user" }
func (m *UserModule) Install(app *astra.App) error {
    g := app.Group("/api/v1/users")
    g.GET("",    m.list)
    g.POST("",   m.create)
    g.GET("/:id", m.get)

    app.OnStop(func(ctx context.Context) error {
        return m.db.Close()
    })
    return nil
}

// main.go — 只负责组装
func main() {
    app := astra.New()
    app.Use(middleware.Recovery(), middleware.Logger())

    app.Register(
        &UserModule{db: mustDB()},
        &OrderModule{db: mustDB()},
    )

    app.Run(":8080")
}
```

**DI 容器**（可选）— 当模块间依赖复杂时引入：

```go
c := di.New()

di.Provide[*sql.DB](c, func(c *di.Container) (*sql.DB, error) {
    return sql.Open("postgres", os.Getenv("DATABASE_URL"))
})

di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
    db, _ := di.Invoke[*sql.DB](c)
    return NewUserService(db), nil
})

c.BindApp(app) // 将 DI 生命周期接入 app 的优雅关闭
```

**Plugin**（可选）— 用于封装第三方集成（Prometheus、Jaeger 等），与 Module 的区别仅在于语义：Plugin 是横切关注点，Module 是业务域。

完整示例：[`examples/basic`](../../examples/basic/main.go)

---

## 概念对照表

| 概念 | 必须掌握 | 等价于 |
|------|---------|--------|
| `astra.New()` + `app.GET/POST` + `app.Run` | ✅ 第一步 | Gin `r := gin.New()` |
| `app.Group` + `app.Use` | ✅ 第一步 | Gin `r.Group` |
| `OnStart` / `OnStop` | ⬜ 有数据库时 | 自定义启停逻辑 |
| `Module` | ⬜ 多文件时 | — |
| `Plugin` | ⬜ 集成第三方时 | — |
| `di.Container` | ⬜ 依赖图复杂时 | Wire / Uber fx |

---

## 下一步

- [项目结构建议](project-structure.md)
- [中间件列表](../guides/middleware.md)
- [错误处理](../guides/error-handling.md)
- [模块系统](../guides/modules.md)
