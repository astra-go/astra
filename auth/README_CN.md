# Auth — 认证与授权

提供 OAuth2 登录和基于 Casbin 的 RBAC 授权支持。

## 特性

- **OAuth2**：支持带 PKCE 的 Authorization Code 流程、OIDC UserInfo 获取、Token 自动刷新
- **CSRF 防护**：HMAC-SHA256 签名 state 存储在 HttpOnly Cookie 中，防止 CSRF 攻击
- **RBAC**：基于 Casbin 的中间件式授权，支持自定义 subject/resource/action 提取器
- **可插拔**：Token 持久化和认证后重定向逻辑均可自定义

## 快速开始

### OAuth2 登录

```go
import (
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    astraoauth2 "github.com/astra-go/astra/auth/oauth2"
)

cfg := astraoauth2.Config{
    ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
    ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
    RedirectURL:  "https://myapp.com/auth/callback",
    Scopes:       []string{"openid", "email", "profile"},
    Endpoint:     google.Endpoint,
    PKCE:         true,
    OnSuccess: func(c *astra.Ctx, tok *oauth2.Token, info map[string]any) error {
        // 存储 Token 并重定向
        return c.Redirect(http.StatusFound, "/dashboard")
    },
}

app.GET("/auth/login",    astraoauth2.LoginHandler(cfg))
app.GET("/auth/callback", astraoauth2.CallbackHandler(cfg))
```

### RBAC 授权中间件

```go
import (
    "github.com/casbin/casbin/v2"
    "github.com/astra-go/astra/auth/rbac"
)

e, _ := casbin.NewEnforcer("model.conf", "policy.csv")
app.Use(rbac.Middleware(rbac.Config{Enforcer: e}))
```

## OAuth2 API

### Config

```go
type Config struct {
    ClientID, ClientSecret string       // OAuth2 应用凭证
    RedirectURL            string       // 回调 URL
    Scopes                 []string     // 请求的权限范围
    Endpoint               oauth2.Endpoint
    PKCE                   bool         // 启用 PKCE（推荐）
    UserInfoURL            string       // OIDC UserInfo 端点
    OnSuccess              func(*astra.Ctx, *oauth2.Token, map[string]any) error
}
```

### LoginHandler / CallbackHandler

```go
// 发起 OAuth2 登录
LoginHandler(cfg Config) astra.HandlerFunc

// 处理回调
CallbackHandler(cfg Config) astra.HandlerFunc
```

### 其他工具

```go
// 刷新过期 Token
RefreshToken(ctx, cfg, expiredToken) (*oauth2.Token, error)

// 获取用户信息
FetchUserInfo(ctx, cfg, token) (map[string]any, error)
```

## RBAC API

### Config

```go
type Config struct {
    Enforcer    *casbin.Enforcer    // Casbin 策略执行器（必填）
    GetSubject  func(*astra.Ctx) string  // 从请求提取 subject，默认："user_id"
    GetObject   func(*astra.Ctx) string  // 提取资源，默认：URL 路径
    GetAction   func(*astra.Ctx) string  // 提取操作，默认：HTTP 方法
    Skipper     func(*astra.Ctx) bool    // 跳过检查函数
    ErrorHandler astra.HandlerFunc       // 自定义 403 响应处理器
}
```

### Subject 提取器示例（配合 JWT）

```go
app.Use(rbac.Middleware(rbac.Config{
    Enforcer: e,
    GetSubject: func(c *astra.Ctx) string {
        uid, _ := c.Get("user_id")
        return fmt.Sprintf("%v", uid)
    },
}))
```

## 配置

### OAuth2 PKCE 模式

| 参数 | 类型 | 默认值 | 说明 |
|-----------|------|---------|-------------|
| `PKCE` | `bool` | `false` | 生产环境推荐启用 S256 PKCE |

### RBAC Skipper

```go
// 跳过健康检查路由
Skipper: func(c *astra.Ctx) bool {
    return c.Path() == "/health" || c.Path() == "/ready"
}
```

## 模块依赖

- `golang.org/x/oauth2` — OAuth2/OIDC 核心库
- `github.com/casbin/casbin/v2` — RBAC 策略引擎

## 注意事项

- OAuth2 回调 URL 必须与 Provider 注册的 Redirect URL 完全一致
- PKCE 模式强制使用 S256 方法；不支持纯 redirect_uri 隐式授权
- Casbin 策略文件变更后，调用 `e.LoadPolicy()` 热更新无需重启