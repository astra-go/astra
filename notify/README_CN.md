# Notify — 统一通知系统

统一的通知发送接口，支持邮件、短信和推送通知。

## 特性

- **邮件**：SMTP 协议发送
- **短信**：阿里云、腾讯云短信
- **推送**：Firebase Cloud Messaging（FCM）
- **统一接口**：所有渠道实现相同接口；业务代码与渠道解耦

## 快速开始

### 邮件通知

```go
import "github.com/astra-go/astra/notify"

email := notify.NewEmailSMTP(notify.SMTPConfig{
    Host:     "smtp.example.com",
    Port:     587,
    Username: "user",
    Password: "pass",
    From:     "noreply@example.com",
})

err := email.Send(ctx, notify.Email{
    To:      "user@example.com",
    Subject: "Registration verification code",
    Body:    "Your verification code is 123456, valid for 5 minutes.",
})
```

### 短信通知

```go
// 阿里云短信
sms := notify.NewSMSAliyun(notify.AliyunSMSConfig{
    AccessKey:  "...",
    SecretKey:  "...",
    SignName:   "MyApp",
    Template:   "SMS_123456",
})

err := sms.Send(ctx, notify.SMS{
    Phone:  "13800138000",
    Params: `{"code":"1234"}`,
})

// 腾讯云短信
sms2 := notify.NewSMSTencent(notify.TencentSMSConfig{
    SecretID:  "...",
    SecretKey: "...",
    AppID:     "...",
    SignName:  "MyApp",
    Template:  "123456",
})
```

### FCM 推送

```go
push := notify.NewPushFCM(notify.FCMConfig{
    ServerKey: "...",
})

err := push.Send(ctx, notify.Push{
    Token: "device-fcm-token",
    Title: "You have a new message",
    Body:  "You have a new order pending processing",
})
```

## API

### 邮件

```go
// SMTP 配置
type SMTPConfig struct {
    Host     string
    Port     int     // 通常 587（TLS）或 465（SSL）
    Username string
    Password string
    From     string
    TLS     bool    // 强制 TLS
}

// 邮件结构
type Email struct {
    To          string            // 收件人
    Cc          []string          // 抄送
    Bcc         []string          // 密送
    Subject     string            // 主题
    Body        string            // 纯文本正文
    HTMLBody    string            // HTML 正文（优先于 Body）
    Headers     map[string]string // 额外头信息
}

// 发送者接口
type Sender interface {
    Send(ctx context.Context, msg any) error
}
```

### 短信

```go
// 阿里云
type AliyunSMSConfig struct {
    AccessKey, SecretKey string
    SignName             string
    Template             string
}

type SMS struct {
    Phone  string            // 手机号
    Params map[string]string // 模板变量
}

// 腾讯云
type TencentSMSConfig struct {
    SecretID, SecretKey string
    AppID, SignName    string
    Template           string
}
```

### 推送

```go
type Push struct {
    Token string            // 设备 FCM Token
    Title string            // 通知标题
    Body  string            // 通知内容
    Data  map[string]string // 额外数据
    Badge int               // iOS 角标数
}
```

## 完整示例

```go
package main

import (
    "context"
    "github.com/astra-go/astra/notify"
)

func main() {
    ctx := context.Background()

    // 邮件
    email := notify.NewEmailSMTP(notify.SMTPConfig{
        Host:     "smtp.gmail.com",
        Port:     587,
        Username: "my@gmail.com",
        Password: "app-password",
        From:     "my@gmail.com",
    })
    email.Send(ctx, notify.Email{
        To:      "user@example.com",
        Subject: "Test email",
        Body:    "This is a test email.",
    })

    // FCM 推送
    push := notify.NewPushFCM(notify.FCMConfig{
        ServerKey: "firebase-server-key",
    })
    push.Send(ctx, notify.Push{
        Token: "device-token",
        Title: "New message",
        Body:  "You have a new message",
    })
}
```

## 模块依赖

| 子包 | 依赖 |
|-------------|-----------|
| `notify` | `gopkg.in/gomail.v2`（邮件） |
| `notify` | 阿里云短信 SDK |
| `notify` | 腾讯云短信 SDK |
| `notify` | `github.com/mileusna/fcm`（FCM） |

## 注意事项

- 邮件密码推荐使用应用专用密码或 OAuth2，而非邮箱登录密码
- 短信模板需在平台预注册，否则可能发送失败
- FCM 在中国大陆可能不可用，建议集成极光、友盟等国内推送服务