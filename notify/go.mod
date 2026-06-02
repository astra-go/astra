module github.com/astra-go/astra/notify

// No external dependencies — uses only Go stdlib (net/http, net/smtp).
go 1.25.1

// Standalone notification module.
// SMS (Aliyun/Tencent) and email (SMTP) use pure stdlib HTTP/SMTP — no SDK.
// FCM push uses the HTTP v1 API with a service-account JWT; net/http only.
// No external dependencies are needed for this module.
