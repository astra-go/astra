# 模块系统架构：Module、Plugin 与 DI Container

Astra 提供三种机制来组织应用代码和扩展框架能力。它们职责不同、层次分明，但可以自由组合。

---

## 三者关系一览

```
┌────────────────────────────────────────────────────────────────┐
│                          *astra.App                            │
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Module / Plugin  ——  应用的"安装单元"                   │   │
│  │                                                         │   │
│  │  每个 Module/Plugin 在 Install/Init 里向 App 注册：      │   │
│  │    · 路由        app.GET / app.Group(...)               │   │
│  │    · 中间件      app.Use(...)                           │   │
│  │    · 生命周期    app.OnStart / app.OnStop               │   │
│  │    · 嵌套模块    app.Register(innerModule)              │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              ↑                                 │
│                   Module 内部可使用                             │
│                              ↑                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  di.Container  ——  依赖注入容器                           │   │
│  │                                                         │   │
│  │  管理单例服务的构造、共享、生命周期绑定                     │   │
│  │    di.Provide[*DB](c, factory)                          │   │
│  │    db := di.MustInvoke[*DB](c)                          │   │
│  │    c.BindApp(app)  ← 把容器的 Start/Stop 接入 App        │   │
│  └─────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

**Module / Plugin** 负责"把功能装进 App"；  
**di.Container** 负责"把依赖注入给 Module/Plugin"。  
两层正交、互不依赖，可以单独使用，也可以搭配使用。

---

## 对比表

| 维度 | `Module` | `Plugin` | `di.Container` |
|------|----------|----------|----------------|
| **核心方法** | `Install(app *App) error` | `Init(app *App) error` | `di.Provide` / `di.Invoke` |
| **适用场景** | 应用自身的业务单元（API 层、领域服务等） | 第三方库集成（Prometheus、Swagger、OAuth2 等） | 跨模块共享的单例依赖（DB、Redis、配置等） |
| **注册入口** | `app.Register(m)` | `app.RegisterPlugin(p)` 或 `app.Register(astra.PluginAsModule(p))` | `di.Provide[T](container, factory)` |
| **重复检测** | ✅（名称唯一） | ✅（通过 PluginAsModule 共享同一命名空间） | ✅（类型+名称唯一） |
| **生命周期** | 通过 `app.OnStart/OnStop` 注册 | 通过 `app.OnStart/OnStop` 注册 | `c.OnStart/OnStop` + `c.BindApp(app)` |
| **依赖共享** | 手动传参 | 手动传参 | 自动：`di.MustInvoke[T](c)` |
| **适合规模** | 中大型应用，按功能分模块 | 可复用的库级集成 | 依赖关系复杂的中大型服务 |
| **引入成本** | 零，核心内置 | 零，核心内置 | `github.com/astra-go/astra/di` |

---

## 决策树

```
我想组织一块功能代码，该选哪个？
│
├─ 这是一个 可复用的第三方库适配器
│   （会被多个项目 import）？
│   └─ 是 → Plugin（实现 Init，用 app.RegisterPlugin 注册）
│
├─ 这是 应用自己的业务逻辑单元
│   （用户服务、设备 API、权限中间件…）？
│   └─ 是 → Module（实现 Install，用 app.Register 注册）
│
└─ 我需要在多个 Module/Plugin 之间 共享一个单例
    （数据库连接、Redis 客户端、配置对象…）？
    └─ 是 → di.Container（di.Provide 注册，di.MustInvoke 取用，c.BindApp 绑定生命周期）
```

> **简记**：Plugin = "我是一个库"，Module = "我是一个功能"，DI = "我是一个依赖"。

---

## 典型组合示例

### 场景一：Module 内部直接传参（小型服务，无需 DI）

```go
// main.go
db, _ := sql.Open("postgres", os.Getenv("DATABASE_URL"))

app := astra.New()
app.Register(
    user.NewModule(db),   // Module 把 db 作为构造参数
    order.NewModule(db),
)
app.Run(":8080")
```

适用于依赖关系简单、模块数量少的场景。

---

### 场景二：di.Container 管理依赖，Module 消费（中大型服务）

```go
// main.go
c := di.New()

di.Provide[*sql.DB](c, func(_ *di.Container) (*sql.DB, error) {
    return sql.Open("postgres", os.Getenv("DATABASE_URL"))
})
di.Provide[*redis.Client](c, func(_ *di.Container) (*redis.Client, error) {
    return redis.NewClient(&redis.Options{Addr: "localhost:6379"}), nil
})

app := astra.New()
c.BindApp(app) // 把 DB/Redis 的 Close 接入 App 的优雅关闭

app.Register(
    user.NewModuleFromContainer(c),
    order.NewModuleFromContainer(c),
)
app.Run(":8080")
```

```go
// user/module.go
type Module struct{ c *di.Container }

func NewModuleFromContainer(c *di.Container) astra.Module {
    return &Module{c: c}
}

func (m *Module) Name() string { return "user" }

func (m *Module) Install(app *astra.App) error {
    db := di.MustInvoke[*sql.DB](m.c)
    svc := NewUserService(db)
    g := app.Group("/users")
    g.GET("",    svc.List)
    g.POST("",   svc.Create)
    g.GET("/:id", svc.Get)
    return nil
}
```

---

### 场景三：Plugin 集成第三方库，Module 承载业务

```go
app := astra.New()

// Plugin：第三方库集成（Swagger UI）
app.RegisterPlugin(swagger.New(swagger.Config{
    BasePath: "/docs",
    Title:    "My API",
}))

// Module：自己的业务
app.Register(
    user.NewModule(db),
    order.NewModule(db),
)

app.Run(":8080")
```

或者统一走 `app.Register`（Plugin 和 Module 混合注册）：

```go
app.Register(
    astra.PluginAsModule(swagger.New(swagger.Config{BasePath: "/docs"})),
    user.NewModule(db),
    order.NewModule(db),
)
```

---

### 场景四：三者全部结合（生产级应用）

```go
func main() {
    // 1. 构建 DI 容器
    c := di.New()
    di.Provide[*sql.DB](c, newDB)
    di.Provide[*redis.Client](c, newRedis)
    di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
        db := di.MustInvoke[*sql.DB](c)
        return NewUserService(db), nil
    })

    // 2. 创建 App，绑定容器生命周期
    app := astra.New()
    c.BindApp(app)

    // 3. 注册 Plugin（第三方库）和 Module（业务）
    app.Register(
        astra.PluginAsModule(swagger.New(swagger.Config{BasePath: "/docs"})),
        astra.PluginAsModule(observability.NewModule(observability.Config{
            ServiceName: "my-svc",
            OTELEndpoint: "http://otel:4318",
        })),
        user.NewModuleFromContainer(c),
        order.NewModuleFromContainer(c),
    )

    app.Run(":8080")
}
```

---

## 常见误区

| 误区 | 正确做法 |
|------|----------|
| 在 `Plugin.Init` 里直接 `sql.Open` | 把 DB 放入 `di.Container`，Plugin 只负责注册路由/中间件 |
| 所有代码都放一个巨型 Module | 按领域拆分：`user.Module`、`order.Module`、`payment.Module` |
| 用 Module 实现可复用的第三方集成 | 改用 Plugin，便于其他项目直接 `app.RegisterPlugin` |
| `di.Provide` 了依赖但忘记 `c.BindApp(app)` | 每个应用主函数调用一次 `c.BindApp(app)`，确保 DB/Redis 随 App 优雅关闭 |
| Module 和 Plugin 都注册了同名 | 它们共享命名空间，重复注册会返回错误；检查 `app.HasModule(name)` |
