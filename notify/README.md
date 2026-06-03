# notify

`github.com/astra-go/astra/notify` 提供与底层提供商解耦的通知发送抽象，包含三个通道：

| 通道 | Build Tag | 内置后端 | 类型 |
|------|-----------|---------|------|
| 邮件 | `email` | SMTP | `EmailMessage` / `Sender` |
| 短信 | `sms` | 阿里云、腾讯云 | `SmsMessage` / `Sender` |
| 推送 | `push` | Firebase FCM | `PushMessage` / `Sender` |

所有后端通过 build tags 按需编译，业务代码只依赖接口，后端可随时切换。

---

## 安装

```bash
go get github.com/astra-go/astra/notify
```

> **注意**: 使用时需要指定 build tag（如 `-tags=email`），否则对应通道的代码不会被编译。

---

## 编译标签

```bash
# 编译所有通道
go build -tags=alltags

# 仅编译邮件
go build -tags=email

# 编译多个通道
go build -tags="email,push"
```

---

## 邮件（Email）— Build Tag: `email`

### 接口与数据结构

```go
// notify.EmailSender — 所有邮件后端都实现此接口
type EmailSender interface {
    Send(ctx context.Context, msg *EmailMessage) error
}

type EmailMessage struct {
    From        string           // 可选，覆盖后端默认发件人
    To          []string         // 收件人，必填
    CC          []string         // 抄送
    BCC         []string         // 密送
    ReplyTo     string           // 回复地址
    Subject     string           // 邮件主题，必填
    TextBody    string           // 纯文本正文
    HTMLBody    string           // HTML 正文
    Attachments []EmailAttachment // 附件列表
}

type EmailAttachment struct {
    Filename    string // 文件名
    ContentType string // MIME 类型
    Data        []byte // 文件内容
}
```

### SMTP 后端

```go
import "github.com/astra-go/astra/notify"
```

**Config 字段说明**

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Host` | `string` | — | SMTP 服务器地址（必填） |
| `Port` | `int` | `587` | 端口；465 用于 Implicit TLS |
| `Username` | `string` | — | SMTP 认证用户名 |
| `Password` | `string` | — | SMTP 认证密码 |
| `From` | `string` | — | 默认发件人地址 |
| `ImplicitTLS` | `bool` | `false` | `true` 使用 SMTPS（465） |
| `TLSConfig` | `*tls.Config` | 系统默认 | 自定义 TLS 配置 |
| `DialTimeout` | `time.Duration` | `10s` | 连接超时 |

#### 发送邮件示例

```go
sender := notify.NewSMTPEmailSender(notify.SMTPConfig{
    Host:     "smtp.example.com",
    Port:     587,
    Username: "no-reply@example.com",
    Password: os.Getenv("SMTP_PASSWORD"),
    From:     "no-reply@example.com",
})

err := sender.Send(ctx, &notify.EmailMessage{
    To:       []string{"alice@example.com"},
    Subject:  "验证码",
    TextBody: "您的验证码是 123456，5 分钟内有效。",
})
```

#### 发送 HTML 邮件

```go
err := sender.Send(ctx, &notify.EmailMessage{
    To:       []string{"alice@example.com"},
    Subject:  "欢迎注册",
    TextBody: "欢迎使用我们的服务！",
    HTMLBody: "<h1>欢迎！</h1><p>感谢注册。</p>",
})
```

---

## 短信（SMS）— Build Tag: `sms`

### 接口与数据结构

```go
// notify.SmsSender — 所有短信后端都实现此接口
type SmsSender interface {
    Send(ctx context.Context, msg *SmsMessage) error
}

type SmsMessage struct {
    To           string            // 收件人手机号（E.164 格式）
    Params       map[string]string // 模板变量
    TemplateCode string            // 覆盖后端默认模板 ID
    SignName     string            // 覆盖后端默认签名
}
```

### 阿里云短信

```go
sender := notify.NewAliyunSmsSender(notify.AliyunSmsConfig{
    AccessKeyID:     os.Getenv("ALIYUN_AK_ID"),
    AccessKeySecret: os.Getenv("ALIYUN_AK_SECRET"),
    SignName:        "MyApp",
    TemplateCode:    "SMS_123456789",
})

err := sender.Send(ctx, &notify.SmsMessage{
    To:     "+8613800138000",
    Params: map[string]string{"code": "998877", "minutes": "5"},
})
```

### 腾讯云短信

```go
sender := notify.NewTencentSmsSender(notify.TencentSmsConfig{
    SecretID:   os.Getenv("TC_SECRET_ID"),
    SecretKey:  os.Getenv("TC_SECRET_KEY"),
    AppID:      "1400123456",
    SignName:   "MyApp",
    TemplateID: "1234567",
})

err := sender.Send(ctx, &notify.SmsMessage{
    To: "+8613800138000",
    Params: map[string]string{
        "1": "998877",
        "2": "5",
    },
})
```

---

## 推送通知（Push）— Build Tag: `push`

### 接口与数据结构

```go
// notify.PushSender — 所有推送后端都实现此接口
type PushSender interface {
    Send(ctx context.Context, msg *PushMessage) (*SendResult, error)
    SendBatch(ctx context.Context, msgs []*PushMessage) ([]*SendResult, error)
}

type PushMessage struct {
    Token       string            // 设备注册令牌
    Topic       string            // 主题（与 Token 二选一）
    Title       string            // 通知标题
    Body        string            // 通知正文
    ImageURL    string            // 图片 URL
    Data        map[string]string // 透传数据
    Badge       int               // 角标数字
    Sound       string            // 铃声
    Priority    string            // "normal" 或 "high"
    CollapseKey string            // 折叠键
    TTL         int               // 存活时间（秒）
}

type SendResult struct {
    MessageID string
    Error     error
}
```

### Firebase Cloud Messaging（FCM）

```go
saJSON, _ := os.ReadFile("firebase-service-account.json")

sender, err := notify.NewFCMPushSender(notify.FCMConfig{
    ProjectID:          "my-project-123",
    ServiceAccountJSON: saJSON,
})

result, err := sender.Send(ctx, &notify.PushMessage{
    Token: deviceToken,
    Title: "新订单",
    Body:  "您的订单 #1234 已发货。",
    Data: map[string]string{
        "order_id": "1234",
        "action":   "view_order",
    },
})
```

---

## 构造器列表

| 构造器 | Build Tag | 说明 |
|--------|-----------|------|
| `NewSMTPEmailSender(cfg)` | `email` | SMTP 邮件发送器 |
| `NewAliyunSmsSender(cfg)` | `sms` | 阿里云短信发送器 |
| `NewTencentSmsSender(cfg)` | `sms` | 腾讯云短信发送器 |
| `NewFCMPushSender(cfg)` | `push` | FCM 推送发送器 |

---

## 自定义后端

实现对应接口即可接入任意第三方服务（SendGrid、Twilio、极光推送等）：

```go
// 自定义邮件后端
type SendGridSender struct{ apiKey string }

func (s *SendGridSender) Send(ctx context.Context, msg *notify.EmailMessage) error {
    // 调用 SendGrid API ...
    return nil
}
```

---

## 错误处理

所有 `Send` 方法都是语义透明的：

- **网络/连接错误**：返回 `fmt.Errorf("pkg: ...: %w", err)`，可用 `errors.As` 向下解包
- **API 业务错误**：错误消息包含提供商返回的原始错误码和描述
- **Context 取消**：使用了 `http.NewRequestWithContext`，context 取消会立即中止请求

---

## 相关文档

- [ADR-005: 子模块数量上限策略](../docs/adr/ADR-005-module-count-limit.md)
- [架构优化路线图](../docs/architecture-optimization-roadmap.md)

## 许可证

MIT
