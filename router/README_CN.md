# Router — Radix-Tree 高性能路由

基于方法键控 radix trie 的 HTTP 路由。

## 特性

- **O(k) 时间复杂度**：k = URL 路径字符数，接近常数时间
- **命名参数**：`/:param` 样式参数路由，自动提取到 `c.Param("param")`
- **通配符**：`/static/*filepath` 捕获剩余路径
- **精确匹配优先**：静态路由优先于参数路由，避免 `/users/new` 被 `/:id` 匹配
- **MethodNotAllowed**：自动检测并返回 405 及正确的 Allow 头
- **路由快照**：支持导出/导入路由树，用于测试和文档生成

## 路由语法

| 语法 | 说明 | 示例 |
|--------|-------------|---------|
| `/:param` | 命名参数 | `/:id` → `/123` |
| `/*filepath` | 通配符（捕获剩余路径） | `/static/*filepath` |
| `/:param+` | 参数重复（一个或多个段） | `/:path+` → `/a/b/c` |
| `/:file(.*\.png)` | 正则约束（Go regexp 格式） | — |

## 快速开始

```go
import "github.com/astra-go/astra/router"

r := router.New()

// 静态路由
r.Add(http.MethodGet, "/", indexHandler)
r.Add(http.MethodGet, "/about", aboutHandler)

// 参数路由
r.Add(http.MethodGet, "/users/:id", getUser)

// 正则约束
r.Add(http.MethodGet, "/files/:filename(.*\\.png)", servePNG)
```

## API

### New

```go
func New() *Router
```

### router.Add

```go
func (r *Router) Add(method, path string, handlers ...astra.HandlerFunc) *Router
```

注册路由。`handlers` 是中间件链（最后一个是处理器）。

### router.Handle

```go
func (r *Router) Handle(c *astra.Ctx) error
```

处理请求，典型用法：

```go
app.Use(func(c *astra.Ctx) error {
    return r.Handle(c)
})
```

### c.Param

```go
func (c *astra.Ctx) Param(key string) string
```

获取路由参数值：

```go
app.GET("/users/:id/posts/:post_id", func(c *astra.Ctx) error {
    userID := c.Param("id")
    postID := c.Param("post_id")
    return c.String(200, fmt.Sprintf("user=%s, post=%s", userID, postID))
})
```

### 其他路由方法

```go
r.GET(path, handlers...)   // = r.Add(http.MethodGet, path, handlers...)
r.POST(path, handlers...)
r.PUT(path, handlers...)
r.PATCH(path, handlers...)
r.DELETE(path, handlers...)
r.OPTIONS(path, handlers...)
r.HEAD(path, handlers...)
```

## 配置

### StrictSlash（尾部斜杠重定向）

```go
r := router.New(router.WithStrictSlash(true))
// /users/ 重定向到 /users
```

### WithMaxParamValueLen

```go
r := router.New(router.WithMaxParamValueLen(256))
// 拒绝路径参数值超过 256 字节
```

## 完整示例

```go
package main

import (
    "fmt"
    "net/http"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/router"
)

func main() {
    r := router.New()

    r.GET("/", func(c *astra.Ctx) error {
        return c.String(200, "Home")
    })

    r.GET("/users/:id", func(c *astra.Ctx) error {
        return c.String(200, fmt.Sprintf("User ID: %s", c.Param("id")))
    })

    r.GET("/files/*filepath", func(c *astra.Ctx) error {
        return c.String(200, fmt.Sprintf("File path: %s", c.Param("filepath")))
    })

    app := astra.New()
    app.Use(func(c *astra.Ctx) error {
        return r.Handle(c)
    })

    app.Run(":8080")
}
```

## 模块依赖

无外部依赖。

## 注意事项

- 路由通常通过 `astra.New()` 注册，无需直接使用 router 包
- 通配参数 `*filepath` 获取时不包含前导 `/`
- 同时注册 `/users/new` 和 `/:id` 时，前者优先（精确匹配规则）
- 参数值默认无长度限制；通过 `WithMaxParamValueLen` 限制