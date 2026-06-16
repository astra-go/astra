# Contract — 框架核心接口定义

Astra 框架的核心接口和类型定义，实现框架核心与子模块解耦。

## 特性

- **Context 接口**：定义请求上下文（Request/Response/Params/Binding）；子模块通过此接口与框架通信
- **Binder 接口**：数据绑定抽象，支持 JSON/XML/Form/Query/Path 多源绑定
- **HTTPError**：HTTP 错误类型，支持状态码 + 消息 + 内部错误组合
- **ValidationError**：字段级校验错误类型
- **Repository[T]**：泛型数据访问抽象；业务层依赖此接口而非直接依赖 GORM
- **TxRunner**：事务运行器，支持 Context 传播的事务
- **Stream 接口**：ServerStream / ClientStream / BidiStream 定义服务端推送、客户端推送、双向流

## 核心接口

### Context 接口

`contract.Context` 是 Astra 请求上下文的抽象接口；子包和中间件应依赖此接口而非 `*astra.Ctx`：

```go
type Context interface {
    // 请求/响应
    Request() *http.Request
    Writer() ResponseWriter

    // 路由参数
    Param(key string) string

    // 查询参数
    Query(key string) string
    DefaultQuery(key, defaultValue string) string

    // 表单数据
    PostForm(key string) string
    FormFile(key string) (*multipart.FileHeader, error)

    // 数据绑定
    BindJSON(obj any) error
    BindQuery(obj any) error
    BindPath(obj any) error
    BindForm(obj any) error

    ShouldBindJSON(obj any) error
    ShouldBindQuery(obj any) error

    // ShouldBind vs Bind：ShouldBind 绑定失败时不自动返回错误，适合处理器自行决定响应

    // 中间件链控制
    Next()
    Abort()
    AbortWithStatus(code int)

    // 请求上下文存储（kv）
    Set(key string, val any)
    Get(key string) (any, bool)

    // JSON 响应
    JSON(code int, v any) error
    String(code int, s string) error

    // 解析语言/地区
    Language() string
}
```

### ResponseWriter 接口

```go
type ResponseWriter interface {
    http.ResponseWriter
    Status() int      // 已写入的状态码
    Size() int         // 已写入的字节数
    Written() bool    // WriteHeader 是否已调用
}
```

### HTTPError

```go
// 创建 HTTP 错误
err := contract.NewHTTPError(404, "user not found")

// 附加内部错误（不对外暴露）
err = err.WithInternal(dbErr)

// 类型检查
if errors.Is(err, contract.NewHTTPError(401)) { ... }
```

### ValidationError

```go
// 单字段错误
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

// 批量校验错误
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
    // "field=name: cannot be blank; field=email: format error"
}
```

## 数据绑定 Tag

| Tag | 来源 | 示例 |
|-----|--------|---------|
| `uri:"name"` | URL 路径参数 | `/users/:id` → `id` |
| `query:"name"` | URL 查询参数 | `?page=1` → `page` |
| `form:"name"` | 表单体 | `POST` 表单字段 |
| `header:"name"` | 请求头 | `X-Request-Id` |
| `json:"name"` | JSON 体 | 请求体 |

## 模块依赖

无外部依赖。`contract` 包是 Astra 的基础设施层，定义框架核心抽象。

## 注意事项

- 子包和中间件应使用 `contract.Context` 而非 `*astra.Ctx`，保持与框架解耦
- `ShouldBind` 与 `Bind` 的区别：前者绑定失败时不自动返回错误，适合处理器自行决定响应
- `Validator` 注册在 `github.com/astra-go/astra/binding` 包中