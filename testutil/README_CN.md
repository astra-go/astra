# TestUtil — 应用测试工具

Astra HTTP 路由和响应断言测试工具。

## 特性

- **测试 App 创建**：无需启动真实 HTTP 服务器
- **请求构造**：`GET`、`POST` 等快速构造 HTTP 请求
- **响应断言**：断言状态码、响应体内容、JSON 字段
- **中间件测试**：可在测试 App 中注册中间件后测试
- **Ginkgo 集成**：`testutil` 支持 Ginkgo BDD 风格测试框架

## 快速开始

```go
import (
    "testing"
    "github.com/astra-go/astra/testutil"
)

func TestHello(t *testing.T) {
    app := testutil.NewTestApp()

    app.GET("/hello", func(c *astra.Ctx) error {
        return c.String(200, "Hello, World!")
    })

    ts := testutil.NewServer(t, app)
    ts.GET("/hello").
        AssertStatus(200).
        AssertBodyContains("Hello")
}
```

## API

### NewTestApp

```go
func NewTestApp() *astra.App
```

创建测试 App（不监听端口，不启动服务器）。

### NewServer

```go
func NewServer(t TB, app *astra.App) *httptest.Server
```

创建真实 HTTP 测试服务器（使用 `net/http/httptest`）。

### 请求构造

```go
ts.GET(path string)       *RequestBuilder
ts.POST(path string)      *RequestBuilder
ts.PUT(path string)       *RequestBuilder
ts.DELETE(path string)    *RequestBuilder
ts.PATCH(path string)     *RequestBuilder
ts.OPTIONS(path string)   *RequestBuilder
```

### RequestBuilder

```go
// 设置请求体
r.WithJSON(body)          // JSON body
r.WithForm(kv)           // 表单数据
r.WithHeader(key, val)    // 自定义 Header

// 发送并断言
r.AssertStatus(code int)  *Response
r.AssertJSON(path string, expect any) // 断言 JSON 字段
r.AssertBodyContains(substr string)
r.AssertHeader(key, expectVal string)
```

## 完整示例

```go
package main

import (
    "encoding/json"
    "net/http"
    "strings"
    "testing"

    "github.com/astra-go/astra"
    "github.com/astra-go/astra/testutil"
)

func TestUserAPI(t *testing.T) {
    app := testutil.NewTestApp()

    // 注册路由和中间件
    app.Use(middleware.Logger())
    app.GET("/users/:id", func(c *astra.Ctx) error {
        return c.JSON(200, astra.Map{
            "id":   c.Param("id"),
            "name": "Alice",
        })
    })

    ts := testutil.NewServer(t, app)

    // 测试路由匹配
    ts.GET("/users/42").
        AssertStatus(http.StatusOK).
        AssertJSON("id", "42").
        AssertJSON("name", "Alice")

    // 测试 JSON 请求
    body := `{"email": "alice@example.com"}`
    ts.POST("/users").
        WithHeader("Content-Type", "application/json").
        WithJSON(map[string]string{"email": "alice@example.com"}).
        AssertStatus(http.StatusCreated).
        AssertBodyContains("created")
}

func TestMiddlewareChain(t *testing.T) {
    var called bool
    app := testutil.NewTestApp()

    app.Use(func(next astra.HandlerFunc) astra.HandlerFunc {
        return func(c *astra.Ctx) error {
            called = true
            return next(c)
        }
    })
    app.GET("/test", func(c *astra.Ctx) error {
        return c.String(200, "ok")
    })

    ts := testutil.NewServer(t, app)
    ts.GET("/test").AssertStatus(200)

    if !called {
        t.Error("Middleware was not called")
    }
}
```

## 模块依赖

- `net/http/httptest` — 标准库 HTTP 测试工具

## 注意事项

- `testutil.NewTestApp()` 创建的 App 不注册任何内置中间件；需手动注册
- `testutil.NewServer` 使用真实 HTTP 服务器；适合集成测试
- 单元测试中使用 `NewTestApp()` 直接测试处理器，无需启动服务器