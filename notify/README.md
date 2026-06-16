# Notify — Unified Notification System

Unified notification sending interface supporting email, SMS, and push notifications.

## Features

- **Email**: SMTP protocol sending
- **SMS**: Alibaba Cloud, Tencent Cloud SMS
- **Push**: Firebase Cloud Messaging (FCM)
- **Unified Interface**: All channels implement the same interface; business code decoupled from channels

## Quick Start

### Email Notification

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

### SMS Notification

```go
// Alibaba Cloud SMS
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

// Tencent Cloud SMS
sms2 := notify.NewSMSTencent(notify.TencentSMSConfig{
    SecretID:  "...",
    SecretKey: "...",
    AppID:     "...",
    SignName:  "MyApp",
    Template:  "123456",
})
```

### FCM Push

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

### Email

```go
// SMTP config
type SMTPConfig struct {
    Host     string
    Port     int     // Usually 587 (TLS) or 465 (SSL)
    Username string
    Password string
    From     string
    TLS     bool    // Force TLS
}

// Email structure
type Email struct {
    To          string            // Recipient
    Cc          []string          // CC
    Bcc         []string          // BCC
    Subject     string            // Subject
    Body        string            // Plain text body
    HTMLBody    string            // HTML body (takes priority over Body)
    Headers     map[string]string // Additional headers
}

// Sender interface
type Sender interface {
    Send(ctx context.Context, msg any) error
}
```

### SMS

```go
// Alibaba Cloud
type AliyunSMSConfig struct {
    AccessKey, SecretKey string
    SignName             string
    Template             string
}

type SMS struct {
    Phone  string            // Phone number
    Params map[string]string // Template variables
}

// Tencent Cloud
type TencentSMSConfig struct {
    SecretID, SecretKey string
    AppID, SignName    string
    Template           string
}
```

### Push

```go
type Push struct {
    Token string            // Device FCM Token
    Title string            // Notification title
    Body  string            // Notification content
    Data  map[string]string // Extra data
    Badge int               // iOS badge count
}
```

## Complete Example

```go
package main

import (
    "context"
    "github.com/astra-go/astra/notify"
)

func main() {
    ctx := context.Background()

    // Email
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

    // FCM push
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

## Module Dependencies

| Sub-package | Dependency |
|-------------|-----------|
| `notify` | `gopkg.in/gomail.v2` (email) |
| `notify` | Alibaba Cloud SMS SDK |
| `notify` | Tencent Cloud SMS SDK |
| `notify` | `github.com/mileusna/fcm` (FCM) |

## Notes

- Email password recommends using app-specific password or OAuth2, not email login password
- SMS templates must be pre-registered on the platform, otherwise sending may fail
- FCM may be unavailable in Mainland China; recommend integrating JPush, UMeng, or other domestic push services
