# notify

`github.com/astra-go/astra/notify` 提供与底层提供商解耦的通知发送抽象，包含三个子系统：

| 子包 | 功能 | 内置后端 |
|------|------|----------|
| `notify/email` | 发送邮件 | SMTP |
| `notify/sms` | 发送短信 | 阿里云、腾讯云 |
| `notify/push` | 移动推送通知 | Firebase FCM |

每个子系统都定义了一个 `Sender` 接口。业务代码只依赖接口，后端可随时切换，也可自定义实现。

---

## 安装

```bash
go get github.com/astra-go/astra/notify
```

该模块仅依赖 Go 标准库（`net/http`、`net/smtp`），无外部 SDK 依赖。

---

## 邮件（Email）

### 接口与数据结构

```go
// email.Sender — 所有邮件后端都实现此接口
type Sender interface {
    Send(ctx context.Context, msg *Message) error
}

type Message struct {
    From        string       // 可选，覆盖后端默认发件人
    To          []string     // 收件人，必填
    CC          []string     // 抄送
    BCC         []string     // 密送
    ReplyTo     string       // 回复地址
    Subject     string       // 邮件主题，必填
    TextBody    string       // 纯文本正文
    HTMLBody    string       // HTML 正文（与 TextBody 同时设置时发送 multipart/alternative）
    Attachments []Attachment // 附件列表
}

type Attachment struct {
    Filename    string // 文件名
    ContentType string // MIME 类型，默认 "application/octet-stream"
    Data        []byte // 文件内容
}
```

### SMTP 后端

```go
import (
    "github.com/astra-go/astra/notify/email"
    emailsmtp "github.com/astra-go/astra/notify/email/smtp"
)
```

**Config 字段说明**

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Host` | `string` | — | SMTP 服务器地址（必填） |
| `Port` | `int` | `587` | 端口；465 用于 Implicit TLS |
| `Username` | `string` | — | SMTP 认证用户名 |
| `Password` | `string` | — | SMTP 认证密码 |
| `From` | `string` | — | 默认发件人地址 |
| `ImplicitTLS` | `bool` | `false` | `true` 使用 SMTPS（465），`false` 使用 STARTTLS |
| `TLSConfig` | `*tls.Config` | 系统默认 | 自定义 TLS 配置 |
| `DialTimeout` | `time.Duration` | `10s` | 连接超时 |

#### 发送纯文本邮件

```go
sender := emailsmtp.New(emailsmtp.Config{
    Host:     "smtp.example.com",
    Port:     587,
    Username: "no-reply@example.com",
    Password: os.Getenv("SMTP_PASSWORD"),
    From:     "no-reply@example.com",
})

err := sender.Send(ctx, &email.Message{
    To:       []string{"alice@example.com"},
    Subject:  "验证码",
    TextBody: "您的验证码是 123456，5 分钟内有效。",
})
```

#### 发送 HTML 邮件

```go
err := sender.Send(ctx, &email.Message{
    To:       []string{"alice@example.com"},
    Subject:  "欢迎注册",
    TextBody: "欢迎使用我们的服务！",         // 客户端回退时显示
    HTMLBody: "<h1>欢迎！</h1><p>感谢注册。</p>",
})
```

当 `TextBody` 和 `HTMLBody` 同时设置时，自动发送 `multipart/alternative`。

#### 抄送、密送与回复地址

```go
err := sender.Send(ctx, &email.Message{
    To:      []string{"alice@example.com"},
    CC:      []string{"bob@example.com"},
    BCC:     []string{"admin@example.com"},
    ReplyTo: "support@example.com",
    Subject: "会议通知",
    TextBody: "请查阅附件中的议程。",
})
```

#### 携带附件

```go
pdfData, _ := os.ReadFile("invoice.pdf")

err := sender.Send(ctx, &email.Message{
    To:      []string{"client@example.com"},
    Subject: "发票",
    HTMLBody: "<p>请查收附件中的发票。</p>",
    Attachments: []email.Attachment{
        {
            Filename:    "invoice-2024.pdf",
            ContentType: "application/pdf",
            Data:        pdfData,
        },
    },
})
```

#### 使用 Implicit TLS（端口 465）

```go
sender := emailsmtp.New(emailsmtp.Config{
    Host:        "smtp.example.com",
    Port:        465,
    Username:    "user@example.com",
    Password:    os.Getenv("SMTP_PASSWORD"),
    From:        "user@example.com",
    ImplicitTLS: true,
})
```

---

## 短信（SMS）

### 接口与数据结构

```go
// sms.Sender — 所有短信后端都实现此接口
type Sender interface {
    Send(ctx context.Context, msg *Message) error
}

type Message struct {
    To           string            // 收件人手机号（E.164 格式，如 "+8613800138000"）
    Params       map[string]string // 模板变量，键名含义因提供商而异
    TemplateCode string            // 覆盖后端默认模板 ID，留空使用默认
    SignName     string            // 覆盖后端默认签名，留空使用默认
}
```

手机号需使用 **E.164 格式**（`+` 加国家码，如 `+8613800138000`）。

### 阿里云短信

**前提**：在阿里云控制台创建短信签名和模板，获取 AccessKey。

```go
import (
    "github.com/astra-go/astra/notify/sms"
    smsaliyun "github.com/astra-go/astra/notify/sms/aliyun"
)
```

**Config 字段说明**

| 字段 | 说明 |
|------|------|
| `AccessKeyID` | 阿里云 RAM AccessKey ID（必填） |
| `AccessKeySecret` | 阿里云 RAM AccessKey Secret（必填） |
| `SignName` | 短信签名（默认值，可被 Message.SignName 覆盖） |
| `TemplateCode` | 模板 Code（默认值，可被 Message.TemplateCode 覆盖） |
| `HTTPTimeout` | HTTP 超时，默认 10s |

#### 发送验证码

```go
sender := smsaliyun.New(smsaliyun.Config{
    AccessKeyID:     os.Getenv("ALIYUN_AK_ID"),
    AccessKeySecret: os.Getenv("ALIYUN_AK_SECRET"),
    SignName:        "MyApp",
    TemplateCode:    "SMS_123456789",
})

// 模板内容示例："您的验证码是 ${code}，有效期 ${minutes} 分钟。"
err := sender.Send(ctx, &sms.Message{
    To:     "+8613800138000",
    Params: map[string]string{
        "code":    "998877",
        "minutes": "5",
    },
})
```

#### 一次性覆盖模板或签名

```go
err := sender.Send(ctx, &sms.Message{
    To:           "+8613800138000",
    TemplateCode: "SMS_987654321", // 使用不同模板
    SignName:     "AnotherSign",   // 使用不同签名
    Params:       map[string]string{"name": "Alice"},
})
```

### 腾讯云短信

**前提**：在腾讯云控制台创建 SMS 应用、签名和模板，获取 SecretID/SecretKey。

```go
import (
    "github.com/astra-go/astra/notify/sms"
    smstencent "github.com/astra-go/astra/notify/sms/tencent"
)
```

**Config 字段说明**

| 字段 | 说明 |
|------|------|
| `SecretID` | 腾讯云 SecretID（必填） |
| `SecretKey` | 腾讯云 SecretKey（必填） |
| `AppID` | SMS 应用 ID（SmsSdkAppId，必填） |
| `SignName` | 短信签名 |
| `TemplateID` | 模板 ID（默认值） |
| `Region` | 地域，默认 `ap-guangzhou` |
| `HTTPTimeout` | HTTP 超时，默认 10s |

#### 发送验证码

腾讯云模板变量使用**位置索引**（`"1"`, `"2"`, …），按键名字典序排序后对应模板中的 `{1}`, `{2}`。

```go
sender := smstencent.New(smstencent.Config{
    SecretID:   os.Getenv("TC_SECRET_ID"),
    SecretKey:  os.Getenv("TC_SECRET_KEY"),
    AppID:      "1400123456",
    SignName:   "MyApp",
    TemplateID: "1234567",
    Region:     "ap-guangzhou",
})

// 模板内容示例："您的验证码：{1}，{2}分钟内有效，请勿泄露。"
err := sender.Send(ctx, &sms.Message{
    To: "+8613800138000",
    Params: map[string]string{
        "1": "998877", // 对应模板 {1}
        "2": "5",      // 对应模板 {2}
    },
})
```

---

## 推送通知（Push）

### 接口与数据结构

```go
// push.Sender — 所有推送后端都实现此接口
type Sender interface {
    Send(ctx context.Context, msg *Message) (*SendResult, error)
    SendBatch(ctx context.Context, msgs []*Message) ([]*SendResult, error)
}

type Message struct {
    Token       string            // 设备注册令牌（FCM）或设备 token（APNs）
    Topic       string            // FCM 主题 / APNs topic（与 Token 二选一）
    Title       string            // 通知标题
    Body        string            // 通知正文
    ImageURL    string            // 通知图片 URL（FCM）
    Data        map[string]string // 透传数据（静默推送）
    Badge       int               // 角标数字
    Sound       string            // 铃声文件名，"default" 使用系统默认
    Priority    string            // "normal"（默认）或 "high"（唤醒休眠设备）
    CollapseKey string            // 折叠键，相同键的消息只显示最新一条（FCM）
    TTL         int               // 存活时间（秒），0 使用提供商默认值
}

type SendResult struct {
    MessageID string // 提供商分配的消息 ID
    Error     error  // 非 nil 表示该条消息发送失败
}
```

### Firebase Cloud Messaging（FCM）

**前提**：在 Firebase 控制台创建项目，下载服务账号 JSON 文件。

```go
import (
    "github.com/astra-go/astra/notify/push"
    pushfcm "github.com/astra-go/astra/notify/push/fcm"
)
```

**Config 字段说明**

| 字段 | 说明 |
|------|------|
| `ProjectID` | Firebase 项目 ID（必填） |
| `ServiceAccountJSON` | 服务账号 JSON 文件内容（与 BearerToken 二选一） |
| `BearerToken` | 静态 Bearer token，用于测试或外部 token 管理 |
| `HTTPTimeout` | HTTP 超时，默认 10s |

#### 使用服务账号（推荐）

```go
saJSON, _ := os.ReadFile("firebase-service-account.json")

sender, err := pushfcm.New(pushfcm.Config{
    ProjectID:          "my-project-123",
    ServiceAccountJSON: saJSON,
})
if err != nil {
    log.Fatal(err)
}
```

SDK 自动完成 JWT 签名、OAuth2 令牌交换，并在令牌过期前自动刷新（有效期 1 小时，提前 60 秒刷新）。

#### 发送通知消息

```go
result, err := sender.Send(ctx, &push.Message{
    Token: deviceToken,
    Title: "新订单",
    Body:  "您的订单 #1234 已发货。",
    Data: map[string]string{
        "order_id": "1234",
        "action":   "view_order",
    },
})
if err != nil {
    log.Printf("send error: %v", err)
} else if result.Error != nil {
    log.Printf("delivery error: %v", result.Error)
} else {
    log.Printf("message ID: %s", result.MessageID)
}
```

#### 高优先级唤醒推送

```go
result, err := sender.Send(ctx, &push.Message{
    Token:    deviceToken,
    Title:    "紧急通知",
    Body:     "您有一条重要消息。",
    Priority: "high",
    TTL:      300, // 5 分钟内无法送达则丢弃
})
```

#### 仅透传数据（静默推送）

```go
// 不设置 Title/Body，仅发送 Data，App 在后台处理
result, err := sender.Send(ctx, &push.Message{
    Token: deviceToken,
    Data: map[string]string{
        "type":    "sync",
        "version": "42",
    },
})
```

#### 推送到主题（Topic）

```go
// 向订阅了 "news" 主题的所有设备推送
result, err := sender.Send(ctx, &push.Message{
    Topic: "news",
    Title: "今日头条",
    Body:  "点击查看最新新闻。",
})
```

#### 批量发送

```go
msgs := []*push.Message{
    {Token: token1, Title: "通知1", Body: "内容1"},
    {Token: token2, Title: "通知2", Body: "内容2"},
    {Token: token3, Title: "通知3", Body: "内容3"},
}

results, err := sender.SendBatch(ctx, msgs)
for i, r := range results {
    if r.Error != nil {
        log.Printf("token[%d] 发送失败: %v", i, r.Error)
    }
}
```

> FCM HTTP v1 的免费批量接口已停用，`SendBatch` 内部顺序调用单条接口。

#### 折叠通知（CollapseKey）

```go
// 同一 CollapseKey 的旧通知被新通知替代，设备上只显示最新一条
result, err := sender.Send(ctx, &push.Message{
    Token:       deviceToken,
    Title:       "新消息",
    Body:        "您有 3 条未读消息。",
    CollapseKey: "chat_unread",
})
```

#### 使用静态 Bearer Token（测试用）

```go
sender, err := pushfcm.New(pushfcm.Config{
    ProjectID:   "my-project-123",
    BearerToken: os.Getenv("FCM_TEST_TOKEN"),
})
```

---

## 自定义后端

实现对应接口即可接入任意第三方服务（SendGrid、Twilio、极光推送等）：

```go
// 自定义邮件后端
type SendGridSender struct{ apiKey string }

func (s *SendGridSender) Send(ctx context.Context, msg *email.Message) error {
    // 调用 SendGrid API ...
    return nil
}

// 注入到业务代码
var emailSender email.Sender = &SendGridSender{apiKey: os.Getenv("SENDGRID_KEY")}
```

---

## 在 Astra 应用中注入

推荐通过依赖注入（`di` 包或手动注入）将 Sender 共享到 handler：

```go
// main.go — 初始化
smsSender := smsaliyun.New(smsaliyun.Config{
    AccessKeyID:     os.Getenv("ALIYUN_AK_ID"),
    AccessKeySecret: os.Getenv("ALIYUN_AK_SECRET"),
    SignName:        "MyApp",
    TemplateCode:    "SMS_123456789",
})

app := astra.New()
app.POST("/auth/send-code", func(c astra.Context) error {
    phone := c.Query("phone")
    code  := generateCode()
    saveCode(phone, code)
    return smsSender.Send(c.Request().Context(), &sms.Message{
        To:     phone,
        Params: map[string]string{"code": code},
    })
})
```

---

## 错误处理

所有 `Send` 方法都是语义透明的：

- **网络/连接错误**：返回 `fmt.Errorf("pkg: ...: %w", err)`，可用 `errors.As` 向下解包。
- **API 业务错误**（如 SMTP 认证失败、阿里云返回非 OK Code）：错误消息包含提供商返回的原始错误码和描述，便于排查。
- **Context 取消**：使用了 `http.NewRequestWithContext`，context 取消会立即中止请求。

```go
err := smsSender.Send(ctx, msg)
if err != nil {
    log.Printf("短信发送失败: %v", err)
    // 错误格式示例: "sms/aliyun: API error isv.BUSINESS_LIMIT_CONTROL: 业务限流"
}
```

---

## 性能优化记录

本节记录框架核心路径的性能优化，供后续迭代参考。所有数据来自 Apple M4，Go 1.25，`-benchmem -count=3 -benchtime=2s`。

### 1. 404/405 快速路径（`router.go` + `error_handler.go`）

**问题**：每次未命中路由都会创建一个 `HandlersChain{...}` 切片字面量 + `NewHTTPError()` + `Map{"error":...}` + JSON 编码，合计约 16 allocs/op。

**方案**：

| 措施 | 说明 |
|------|------|
| 预分配 `notFoundChain` / `methodNotAllowedChain` | 在 `newRouter()` 中一次性分配，请求路径直接赋值 |
| 哨兵 `*HTTPError` 指针 | 包级变量 `errDefaultNotFound`，用指针相等替代类型断言 |
| 预构建响应体 | `prebuiltBody404 = []byte(...)` 直接写入连接，跳过 map 创建和 JSON 编码 |

```go
// error_handler.go — 快速路径，0 allocs
if err == errDefaultNotFound {
    writePrebuiltError(ctx, http.StatusNotFound, prebuiltBody404)
    return
}
```

**结果**：

| 指标 | 优化前 | 优化后 |
|------|--------|--------|
| ns/op | 770 | 403 |
| allocs/op | 16 | 9 |

---

### 2. ConsistentHash 环重建（`loadbalance/balancer.go`）

**问题**：节点集合变更时重建需要 86 µs / 1508 allocs。根因：`fmt.Sprintf` 拼接虚节点 key（1500 次堆分配）、`fnv.New32a()` + `[]byte(s)` 各 2 allocs、`strings.Join` 指纹计算、`sort.Slice` 闭包。

**方案**：

| 措施 | 说明 |
|------|------|
| `atomic.Pointer[vnodeRing]` | 稳态读取无锁；互斥锁仅保护环重建 |
| 每实例复用 scratch buffer | `buf := make([]byte, 0, len(inst.ID)+12)` 分配一次，所有副本复用 |
| 内联 FNV-1a | `fnv1a32`/`fnv1a32b`/`fnv1a64` 替代 `fnv.New32a()` + 类型转换 |
| 可交换指纹哈希 | `sum×p1 ^ xor×p2 ^ count×p3`，O(N)、0 allocs，替代 sort+join |
| `slices.SortFunc` | Go 1.21+ pdqsort，替代 `sort.Slice` 闭包 |

```go
// loadbalance/balancer.go — 每实例分配一次 scratch buffer
buf := make([]byte, 0, len(inst.ID)+12)
buf = append(buf, inst.ID...)
buf = append(buf, '#')
base := len(buf)
for i := range c.replicas {
    b := strconv.AppendInt(buf[:base], int64(i), 10)
    ring = append(ring, vnode{hash: fnv1a32b(b), inst: inst})
}
```

**结果**：

| 场景 | 优化前 | 优化后 |
|------|--------|--------|
| 稳态（命中缓存） | 22 ns / 1 alloc | 22 ns / 1 alloc |
| 环重建（N=10 节点） | 86 µs / 1508 allocs | 21 µs / 4 allocs |

---

### 3. JWT 中间件（`middleware/jwt.go` + `middleware/jwt_cache.go`）

**问题**：每次请求约 64 allocs，主要来源：

- `jwt.ParseWithClaims` 内部调用 `jwt.NewParser()` = 1 alloc
- `var mc jwt.MapClaims` + JSON 反序列化 ≈ 45–50 allocs（JWT payload 的字符串键/值）
- `mapClaimsToRegistered` 中三个 `*jwt.NumericDate` 分配
- `*Claims` 结构体分配

**方案**：

#### 3a. 预构建 `*jwt.Parser`

在 `JWTWithConfig` 中一次性创建 Parser，闭包捕获复用：

```go
// 每个 JWTWithConfig 调用只创建一次
parser := jwt.NewParser(jwt.WithExpirationRequired(), jwt.WithLeeway(leeway))

// 每次请求
claims, err = parseToken(raw, parser, keyFunc) // 不再内部 NewParser()
```

节省：1 alloc/请求。

#### 3b. 池化 `jwt.MapClaims`

```go
var jwtMCPool = sync.Pool{
    New: func() any {
        mc := make(jwt.MapClaims, 8)
        return &mc
    },
}

func parseToken(raw string, p *jwt.Parser, keyFunc jwt.Keyfunc) (*Claims, error) {
    mcp := jwtMCPool.Get().(*jwt.MapClaims)
    defer jwtMCPool.Put(mcp)
    for k := range *mcp { delete(*mcp, k) } // 复用前清空

    token, err := p.ParseWithClaims(raw, mcp, keyFunc)
    // ... 提取 claims 后 Put 回 pool
}
```

`json.Unmarshal` 向非 nil map 写入时复用底层哈希表，节省 map header 分配。

#### 3c. 分片 LRU Claims 缓存

对高重复率场景（同一用户 token 反复验证），缓存命中可绕过全部加密和 JSON 解析：

```go
// 启用方式：设置 CacheSize > 0
app.Use(middleware.JWTWithConfig(middleware.JWTConfig{
    Secret:    secret,
    CacheSize: 512, // 建议值：512–2048
}))
```

实现特点：
- **16 分片**，各自独立 `sync.RWMutex`，降低锁竞争
- **FNV-1a 路由**，0 allocs 定位分片
- **TTL = token exp**，读时懒过期检查；写满时先删已过期条目，再随机驱逐至半满
- **读路径**：`RLock` → map 查找 → `RUnlock`，无写锁竞争

```go
// middleware/jwt_cache.go
func (c *jwtCache) get(token string, now int64) (*Claims, bool) {
    sh := c.shardFor(token)
    sh.mu.RLock()
    e, ok := sh.entries[token]
    sh.mu.RUnlock()
    if !ok || e.expireAt <= now {
        return nil, false
    }
    return e.claims, true
}
```

> **安全注意**：缓存绕过逐请求签名验证。密钥轮换后缓存中的旧签名 token 在过期前仍有效；不兼容 token 撤销（黑名单）场景。如需撤销，请保持 `CacheSize: 0`。

**结果**：

| 基准 | ns/op | allocs/op | 说明 |
|------|-------|-----------|------|
| `BenchmarkJWT_ValidToken`（无缓存，旧） | ~2500 | ~64 | 优化前基线 |
| `BenchmarkJWT_ValidToken`（无缓存，新） | 2091 | **53** | 预构建 Parser + 池化 MapClaims |
| `BenchmarkJWT_CacheHit` | **322** | **4** | 缓存命中，全部 JWT 开销为零 |

缓存命中路径相比无缓存减少 **93% 分配**，提速 **6.5×**。4 allocs 为 `httptest.ResponseRecorder` 固有开销，与 JWT 无关。
