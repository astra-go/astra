# 从 Gin / Echo 迁移到 Astra

Astra 的 API 设计刻意对齐 Gin/Echo，大多数代码可以直接复制粘贴，只需调整少数几处。

---

## 5 分钟对照表

### 应用初始化

| Gin | Echo | Astra |
|-----|------|-------|
| `gin.New()` | `echo.New()` | `astra.New()` |
| `gin.Default()` | — | `astra.New()` + `app.Use(middleware.Logger(), middleware.Recovery())` |
| `r.Run(":8080")` | `e.Start(":8080")` | `app.Run(":8080")` |

### 路由注册

| Gin | Echo | Astra |
|-----|------|-------|
| `r.GET("/path", h)` | `e.GET("/path", h)` | `app.GET("/path", h)` |
| `r.POST("/path", h)` | `e.POST("/path", h)` | `app.POST("/path", h)` |
| `r.Group("/api")` | `e.Group("/api")` | `app.Group("/api")` |
| `g.Use(mw)` | `g.Use(mw)` | `g.Use(mw)` |

### Handler 签名

这是最主要的差异：Astra 的 Handler 返回 `error`，和 Echo 一致，与 Gin 不同。

```go
// Gin
func handler(c *gin.Context) {
    c.JSON(200, gin.H{"ok": true})
    // 错误时需要手动 c.Abort()
}

// Echo
func handler(c echo.Context) error {
    return c.JSON(200, map[string]any{"ok": true})
}

// Astra — 和 Echo 几乎一样
func handler(c *astra.Ctx) error {
    return c.JSON(200, astra.Map{"ok": true})
}
```

### Context 方法

| Gin | Echo | Astra |
|-----|------|-------|
| `c.Param("id")` | `c.Param("id")` | `c.Param("id")` ✅ 完全一致 |
| `c.Query("q")` | `c.QueryParam("q")` | `c.Query("q")` |
| `c.DefaultQuery("q", "def")` | — | `c.DefaultQuery("q", "def")` |
| `c.PostForm("field")` | `c.FormValue("field")` | `c.PostForm("field")` |
| `c.GetHeader("X-Foo")` | `c.Request().Header.Get("X-Foo")` | `c.Header("X-Foo")` |
| `c.SetHeader("X-Foo", "bar")` | `c.Response().Header().Set(...)` | `c.SetHeader("X-Foo", "bar")` |
| `c.ClientIP()` | `c.RealIP()` | `c.ClientIP()` |
| `c.Set("key", val)` | `c.Set("key", val)` | `c.Set("key", val)` ✅ 完全一致 |
| `c.Get("key")` | `c.Get("key")` | `c.Get("key")` ✅ 完全一致 |
| `c.Request` | `c.Request()` | `c.Request()` |

### 请求绑定

| Gin | Echo | Astra |
|-----|------|-------|
| `c.ShouldBindJSON(&req)` | `c.Bind(&req)` | `c.BindJSON(&req)` |
| `c.ShouldBind(&req)` | `c.Bind(&req)` | `c.Bind(&req)` |
| `c.ShouldBindQuery(&req)` | `c.Bind(&req)` | `c.BindQuery(&req)` |
| `c.ShouldBindUri(&req)` | — | `c.BindPath(&req)` |

### 响应写入

| Gin | Echo | Astra |
|-----|------|-------|
| `c.JSON(200, obj)` | `c.JSON(200, obj)` | `c.JSON(200, obj)` ✅ 完全一致 |
| `c.String(200, "text")` | `c.String(200, "text")` | `c.String(200, "text")` ✅ 完全一致 |
| `c.XML(200, obj)` | `c.XML(200, obj)` | `c.XML(200, obj)` ✅ 完全一致 |
| `c.Redirect(302, url)` | `c.Redirect(302, url)` | `c.Redirect(302, url)` ✅ 完全一致 |
| `c.File("path")` | `c.File("path")` | `c.File("path")` ✅ 完全一致 |
| `c.Status(204)` | `c.NoContent(204)` | `c.NoContent(204)` |
| `c.AbortWithStatus(403)` | `return c.NoContent(403)` | `c.AbortWithStatus(403); return nil` |

### 中间件写法

这是第二个主要差异：Astra 中间件调用 `return next(c)` 而不是 `c.Next()`。

```go
// Gin 中间件
func myMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 前置逻辑
        c.Next()
        // 后置逻辑
    }
}

// Echo 中间件
func myMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        // 前置逻辑
        err := next(c)
        // 后置逻辑
        return err
    }
}

// Astra 中间件 — 和 Echo 几乎一样
func myMiddleware(next astra.HandlerFunc) astra.HandlerFunc {
    return func(c *astra.Ctx) error {
        // 前置逻辑
        err := next(c)
        // 后置逻辑
        return err
    }
}
```

### 错误处理

Astra 的错误处理比 Gin 更统一：

```go
// Gin — 需要手动写响应并 Abort
func handler(c *gin.Context) {
    user, err := getUser(id)
    if err != nil {
        c.JSON(404, gin.H{"error": "not found"})
        c.Abort()
        return
    }
    c.JSON(200, user)
}

// Astra — 直接返回 error，框架统一处理
func handler(c *astra.Ctx) error {
    user, err := getUser(id)
    if err != nil {
        return astra.NewHTTPError(404, "not found")
    }
    return c.JSON(200, user)
}
```

自定义错误处理器：

```go
app := astra.New(
    astra.WithErrorHandler(func(c *astra.Ctx, err error) {
        var he *astra.HTTPError
        if errors.As(err, &he) {
            c.JSON(he.Code, astra.Map{"error": he.Message})
            return
        }
        c.JSON(500, astra.Map{"error": "internal server error"})
    }),
)
```

---

## 完整迁移示例

### 迁移前（Gin）

```go
package main

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

type CreateItemReq struct {
    Name  string `json:"name"  binding:"required,max=64"`
    Price int    `json:"price" binding:"gte=0"`
}

func main() {
    r := gin.Default()

    v1 := r.Group("/api/v1")
    v1.GET("/items", listItems)
    v1.POST("/items", createItem)

    r.Run(":8080")
}

func listItems(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"items": []any{}})
}

func createItem(c *gin.Context) {
    var req CreateItemReq
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusCreated, req)
}
```

### 迁移后（Astra）

```go
package main

import (
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
    app.Use(middleware.Logger(), middleware.Recovery())

    v1 := app.Group("/api/v1")
    v1.GET("/items", listItems)
    v1.POST("/items", createItem)

    app.Run(":8080")
}

func listItems(c *astra.Ctx) error {
    return c.JSON(http.StatusOK, astra.Map{"items": []any{}})
}

func createItem(c *astra.Ctx) error {
    var req CreateItemReq
    if err := c.BindJSON(&req); err != nil {
        return err // 框架统一处理，返回 400
    }
    return c.JSON(http.StatusCreated, req)
}
```

改动清单：
1. `gin.Default()` → `astra.New()` + `app.Use(middleware.Logger(), middleware.Recovery())`
2. `*gin.Context` → `*astra.Ctx`
3. Handler 签名加 `error` 返回值
4. `c.ShouldBindJSON` → `c.BindJSON`
5. `binding:"..."` 标签 → `validate:"..."` 标签
6. 错误时直接 `return err`，不再需要 `c.Abort()`
7. `gin.H` → `astra.Map`（完全等价）

---

## 不需要学的东西

以下功能完全可选，只有在需要时才引入：

| 功能 | 何时需要 |
|------|---------|
| **Module / Plugin** | 项目拆多文件、需要可复用的功能单元时 |
| **DI 容器** | 依赖关系复杂（DB → Service → Handler）时 |
| **Reactor 引擎** | 空闲连接数 > 10 万、需要降低 goroutine 开销时 |
| **生命周期钩子** | 需要在启动/关闭时执行初始化/清理逻辑时 |

对于从 Gin/Echo 迁移的简单项目，只需掌握上面的对照表即可。

---

## 常见问题

**Q：我的 Gin 中间件能直接用吗？**

不能直接用，因为类型签名不同。但迁移很简单，参考上面的中间件写法对照表。

**Q：`gin.H` 和 `astra.Map` 一样吗？**

完全一样，都是 `map[string]any` 的类型别名。

**Q：Astra 支持 `http.Handler` 吗？**

支持。`*astra.App` 实现了 `http.Handler` 接口，可以直接传给 `http.ListenAndServe`。

**Q：我能在 Astra 中使用 Gin 的中间件生态吗？**

不能直接使用，但 Astra 内置了 Gin 生态中最常用的中间件（Logger / Recovery / CORS / JWT / RateLimit / CSRF 等），通常不需要引入第三方中间件。
