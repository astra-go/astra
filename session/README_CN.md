# Session — HTTP Session 管理

插件化 HTTP Session 管理。当前实现使用 Redis 存储。

## 特性

- **数据安全**：Cookie 只存储 HMAC-SHA256 签名的 Session ID，不暴露任何用户数据
- **防篡改**：签名验证确保客户端无法伪造 Session ID
- **Session 固定攻击防护**：登录后调用 `RegenerateID()` 获取新 Session ID
- **自动保存**：脏 Session 在请求处理后自动写回存储
- **多存储后端**：目前提供 Redis 存储；其他存储可实现接口

## 快速开始

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/session"
    sessredis "github.com/astra-go/astra/session/redis"
)

store := sessredis.New(redisClient, sessredis.Config{
    KeyPrefix: "sess:",
    SecretKey: "your-secret-key-min-32bytes!!",
})

app.Use(session.Middleware(store))

// 登录
app.POST("/login", func(c *astra.Ctx) error {
    sess := session.Get(c)
    sess.Set("user_id", 42)
    return c.JSON(200, astra.Map{"ok": true})
})

// 获取登录状态
app.GET("/profile", func(c *astra.Ctx) error {
    sess := session.Get(c)
    uid, ok := sess.GetInt("user_id")
    if !ok {
        return astra.NewHTTPError(401, "not authenticated")
    }
    return c.JSON(200, astra.Map{"user_id": uid})
})
```

## API

### session.Get

```go
func Get(c contract.Context) Session
```

从请求 Context 获取当前 Session。

### Session 接口

```go
type Session interface {
    ID() string                        // 获取 Session ID
    Get(key string) (any, bool)       // 获取值
    GetInt(key string) (int, bool)   // 获取整型
    Set(key string, val any)          // 设置值
    Delete(key string)                // 删除 key
    Clear()                           // 清空所有 key
    RegenerateID() error               // 重新生成 Session ID（防止固定攻击）
    Destroy() error                    // 销毁 Session（登出）
    Save() error                       // 手动保存
}
```

### 配置选项

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `KeyPrefix` | `string` | `"sess:"` | Redis key 前缀 |
| `SecretKey` | `string` | — | HMAC 签名密钥（≥32 字节） |
| `MaxAge` | `time.Duration` | `24h` | Cookie 有效期 |
| `CookieName` | `string` | `"session_id"` | Cookie 名称 |
| `Domain` | `string` | `""` | Cookie 域名 |
| `Secure` | `bool` | `true` | 仅 HTTPS |
| `HTTPOnly` | `bool` | `true` | 阻止 JavaScript 访问 |
| `SameSite` | `http.SameSite` | `LaxMode` | CSRF 防护级别 |

## 完整示例

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/session"
    sessredis "github.com/astra-go/astra/session/redis"
)

func main() {
    store := sessredis.New(redisClient, sessredis.Config{
        KeyPrefix: "sess:",
        SecretKey: "change-this-secret-key-32bytes!",
    })

    app := astra.New()
    app.Use(session.Middleware(store))

    app.POST("/login", func(c *astra.Ctx) error {
        sess := session.Get(c)
        sess.Set("user_id", 42)
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.POST("/logout", func(c *astra.Ctx) error {
        sess := session.Get(c)
        sess.Destroy()
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.GET("/profile", func(c *astra.Ctx) error {
        sess := session.Get(c)
        uid, ok := sess.GetInt("user_id")
        if !ok {
            return astra.NewHTTPError(401, "not authenticated")
        }
        return c.JSON(200, astra.Map{"user_id": uid})
    })

    app.Run(":8080")
}
```

## 模块依赖

- `github.com/redis/go-redis/v9` — Redis 客户端

## 注意事项

- `SecretKey` 必须是随机字符串 ≥32 字节；切勿硬编码或提交到仓库
- 登录成功后必须调用 `RegenerateID()` 防止 Session 固定攻击
- `Destroy()` 后无需手动清除 Cookie — 中间件在响应时自动处理