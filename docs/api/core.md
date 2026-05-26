# 核心 API — App / Context / Router

> **稳定性：Stable**（v1.0+）

---

## App

`App` 是框架的核心入口，持有路由树、中间件链和生命周期钩子。

### 创建

```go
import "github.com/astra-go/astra"

app := astra.New()                          // 默认选项
app := astra.New(
    astra.WithLogger(myLogger),
    astra.WithTrustedProxies("10.0.0.0/8", "172.16.0.0/12"),
    astra.WithMaxMultipartMemory(32 << 20), // 32 MB
)
```

#### `Option` 函数

| 函数 | 说明 |
|------|------|
| `WithLogger(l *slog.Logger)` | 替换默认 `slog.Default()` |
| `WithTrustedProxies(cidr ...string)` | 设置可信代理 CIDR（`ClientIP()` XFF 遍历用） |
| `WithMaxMultipartMemory(n int64)` | multipart 内存限制，默认 32 MB |
| `WithErrorHandler(fn ErrorHandlerFunc)` | 替换全局错误处理器 |

---

### 路由注册

```go
app.GET("/users",          listUsers)
app.POST("/users",         createUser)
app.PUT("/users/:id",      updateUser)
app.DELETE("/users/:id",   deleteUser)
app.PATCH("/users/:id",    patchUser)
app.HEAD("/users/:id",     headUser)
app.OPTIONS("/users",      optionsUsers)
app.Any("/ws",             handleWebSocket)  // 注册所有方法
```

#### 路径模式

| 模式 | 示例 | 匹配 |
|------|------|------|
| 静态 | `/about` | `/about` |
| 参数 | `/users/:id` | `/users/42` |
| 通配 | `/files/*path` | `/files/a/b/c` |
| 正则 | `/order/:id([0-9]+)` | `/order/123`（数字） |

---

### 中间件

```go
// 全局
app.Use(middleware.Logger(), middleware.Recovery())

// 分组
v1 := app.Group("/v1", middleware.JWT(jwtCfg))
v1.GET("/profile", getProfile)

// 单路由
app.GET("/admin", adminOnly, adminHandler)
```

---

### 静态文件

```go
app.Static("/assets", "./public")
// GET /assets/style.css → ./public/style.css
```

---

### Module 与 Plugin

Astra 提供两种扩展接口，职责不同，但都通过统一的注册路径安装到 App：

| 接口 | 方法 | 适用场景 |
|------|------|----------|
| `Module` | `Install(app *App) error` | 应用自身的业务单元（API 层、领域服务等） |
| `Plugin` | `Init(app *App) error` | 第三方库集成（Prometheus、Swagger 等） |

```go
// Module：应用业务逻辑
type APIModule struct{ db *sql.DB }
func (m *APIModule) Name() string { return "api" }
func (m *APIModule) Install(app *astra.App) error {
    app.GET("/users", m.listUsers)
    return nil
}
app.Register(&APIModule{db: db})

// Plugin：第三方库集成
app.RegisterPlugin(swagger.New(swagger.Config{BasePath: "/docs"}))

// 混合注册（统一走 Register，两者共享命名空间）
app.Register(
    astra.PluginAsModule(swagger.New(swagger.Config{BasePath: "/docs"})),
    &APIModule{db: db},
)
```

如何在 Module、Plugin 和 DI Container 之间选择？参见 [架构指南：Module / Plugin / DI Container](../guides/architecture.md)。

---

### 生命周期

```go
app.OnStart(func(ctx context.Context) error {
    return db.Connect(ctx)
})
app.OnStop(func(ctx context.Context) error {
    return db.Close(ctx)
})
```

钩子按注册顺序**串行**执行，并传入带截止时间的 context。

---

### 启动方法

| 方法 | 说明 |
|------|------|
| `Run(addr)` | HTTP/1.1 + HTTP/2（h2c） |
| `RunTLS(addr, cert, key)` | HTTPS（TLS 1.2+） |
| `RunQUIC(addr, cert, key)` | HTTP/3（QUIC）+ TLS 升级 Alt-Svc |
| `RunServer(srv *http.Server)` | 自定义 `http.Server` |
| `RunReactor(addr)` | Reactor 模式（epoll/kqueue） |
| `ServeHTTP(w, r)` | 实现 `http.Handler`，可嵌入任意 HTTP 服务 |

所有启动方法均在收到 `SIGINT` / `SIGTERM` 后执行**优雅关闭**（30 s 超时）。

---

## Context

`Context` 包装 `http.ResponseWriter` + `*http.Request`，提供框架所有的
请求/响应便捷方法。

### 别名

```go
type Context = Ctx   // *astra.Context 是 *astra.Ctx 的别名
```

---

### 请求读取

```go
// 路径参数
id := c.Param("id")               // string

// 查询字符串
q  := c.Query("q")                // string
q  := c.QueryDefault("q", "all")  // string，带默认值
qs := c.QueryArray("tag")         // []string

// Header
token := c.Header("Authorization") // string

// Cookie
v, err := c.Cookie("session")

// 原始请求
r := c.Request()   // *http.Request
```

---

### 绑定与校验

```go
type CreateUserReq struct {
    Name  string `json:"name"  validate:"required,max=64"`
    Email string `json:"email" validate:"required,email"`
}

var req CreateUserReq
if err := c.ShouldBind(&req); err != nil {
    return err   // 返回 *ValidationError，框架自动渲染 422
}
// MustBind：绑定失败则 panic（仅测试环境使用）
c.MustBind(&req)
```

`ShouldBind` 按 `Content-Type` 自动选择解码器（JSON/XML/YAML/TOML/Form）；
结构体字段支持 `form:`, `query:`, `json:`, `header:`, `cookie:` 标签。

---

### 响应渲染

```go
c.JSON(200, data)                     // application/json
c.XML(200, data)                      // application/xml
c.String(200, "hello %s", name)       // text/plain
c.HTML(200, "<h1>%s</h1>", title)     // text/html
c.Blob(200, "image/png", bytes)       // 二进制
c.File("/path/to/file.pdf")           // 文件下载（Content-Disposition）
c.Stream(200, "text/event-stream", r) // 流式响应
c.NoContent(204)                      // 无响应体
c.Redirect(302, "/login")
```

---

### 错误返回

```go
// 在 handler 中直接 return error，框架统一处理
return astra.NewHTTPError(404, "user not found")
return astra.NewHTTPError(422, astra.H{"field": "email", "msg": "invalid"})

// 带业务码
return astra.NewAppError("USER_NOT_FOUND", 404, "用户不存在")
```

---

### 上下文存储

```go
c.Set("user", user)
v := c.Get("user")          // any
u := c.MustGet("user")      // any，key 不存在时 panic
```

---

### IP / 网络

```go
ip  := c.ClientIP()         // 穿透可信代理的真实客户端 IP
raw := c.RealIP()           // RemoteAddr（原始连接 IP）
```

---

### Writer 直接操作

```go
w := c.Writer()             // http.ResponseWriter
w.Header().Set("X-Custom", "v")
w.WriteHeader(200)
fmt.Fprint(w, "raw output")
```

---

## Router

路由器使用**基数树（Radix Tree）** 实现，每个 HTTP 方法对应独立的树。

### 冲突检测

v0.9+ 起，向同一 method+path 注册两次处理函数会触发 `slog.Warn` 日志：

```
WARN  astra: route conflict: handler overwritten  method=GET path=/users/:id
```

新注册的处理函数**覆盖**旧的。同一路径不同方法不产生冲突。

### HandlersChain

每个路由对应一个 `HandlersChain`（`[]HandlerFunc`），框架按顺序调用，
任何一个返回非 nil error 则中断链并交给错误处理器。

```go
type HandlerFunc    func(c *Context) error
type MiddlewareFunc func(c *Context) error   // 同类型，语义上表示"不是终端"
type HandlersChain  []HandlerFunc
```

---

## 错误类型

| 类型 | 构造函数 | 说明 |
|------|----------|------|
| `*HTTPError` | `NewHTTPError(code, msg...)` | HTTP 状态码 + 消息 |
| `*AppError` | `NewAppError(bizCode, httpStatus, msg)` | 业务码 + HTTP 状态 |
| `ValidationErrors` | 由 `ShouldBind` 自动产生 | 字段级校验错误列表 |

```go
var he *astra.HTTPError
if errors.As(err, &he) {
    log.Printf("http %d: %v", he.Code, he.Message)
}

var ae *astra.AppError
if errors.As(err, &ae) {
    log.Printf("biz %s: %v", ae.BizCode, ae.Message)
}
```
