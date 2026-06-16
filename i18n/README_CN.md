# i18n — 国际化

零依赖、线程安全的多语言消息包，内置语言支持且支持运行时扩展。

## 特性

- **内置语言**：en、zh/zh-CN、zh-TW/zh-HK、ja、ko、fr、de、es、pt/pt-BR、ru、ar
- **运行时注册/扩展**：可在启动时注册新语言或覆盖现有翻译
- **上下文感知**：`i18n.T(c, key)` 自动检测当前请求语言
- **中间件集成**：`i18n.Middleware()` 自动从 Accept-Language 头设置语言
- **Go 模板支持**：消息格式遵循 `fmt.Sprintf` 规范，支持 `%s`、`%d` 等

## 快速开始

### 初始化

```go
import "github.com/astra-go/astra/i18n"

// 注册自定义消息（覆盖或添加语言）
i18n.Register("zh", i18n.Messages{
    "validate.required": "%s cannot be blank",
    "http.404":          "Resource not found",
    "common.welcome":    "Welcome to %s",
})
```

### 中间件方式（推荐）

```go
app.Use(i18n.Middleware())

func handler(c *astra.Ctx) error {
    msg := i18n.T(c, "common.success")
    return c.JSON(200, astra.Map{"message": msg})
}
```

### 直接调用

```go
// 手动设置语言
i18n.SetLanguage("ja")
msg := i18n.T(context.Background(), "validate.required")
// 模板参数
msg := i18n.Format(context.Background(), "common.welcome", "Astra")
```

## API

### i18n.T

```go
func T(c contract.Context, key string) string

// 不依赖请求上下文时使用 Background
func T(ctx context.Context, key string) string
```

获取当前语言的消息模板并用 `fmt.Sprintf` 格式化。

### i18n.Format

```go
func Format(ctx context.Context, key string, args ...any) string
```

直接传入模板参数，返回格式化字符串。

### i18n.Register

```go
func Register(locale string, msgs Messages)
```

注册或覆盖某语言的消息字典：

```go
i18n.Register("fr", i18n.Messages{
    "hello": "Bonjour, %s!",
})
```

### i18n.Extend

```go
func Extend(locale string, msgs Messages)
```

向现有语言追加消息（不覆盖已有 key）：

```go
i18n.Extend("zh", i18n.Messages{
    "app.brand": "MyApp",
})
```

### i18n.SetLanguage

```go
func SetLanguage(locale string)
```

设置包级默认语言（后续所有 `T` 调用使用该语言）。

### Bundle（高级）

```go
// 创建独立的多语言包
bundle := i18n.New()
bundle.Register("de", i18n.Messages{"hello": "Hallo"})
bundle.Register("fr", i18n.Messages{"hello": "Bonjour"})
bundle.SetFallback("en")

// 从 bundle 获取翻译器
t := bundle.Tr("de")
msg := t.T("hello")
```

## 完整示例

```go
package main

import (
    "fmt"

    "github.com/astra-go/astra/i18n"
)

func main() {
    // 注册自定义翻译
    i18n.Register("zh", i18n.Messages{
        "validate.required": "%s cannot be blank",
        "order.success":     "Order %s created successfully",
    })

    // 英文
    i18n.SetLanguage("en")
    fmt.Println(i18n.Format(nil, "validate.required", "Email"))
    // 输出: Email cannot be blank

    // 中文
    i18n.SetLanguage("zh")
    fmt.Println(i18n.Format(nil, "validate.required", "邮箱"))
    // 输出: 邮箱 cannot be blank
}
```

## 模块依赖

无外部依赖。

## 注意事项

- `i18n.T(nil, key)` 使用包级默认语言
- 中间件方式下，语言自动从请求的 `Accept-Language` 头解析（取权重最高的项）
- `i18n.T(c, key)` 优先使用 Context 语言，其次使用包级默认语言
- key 未找到时返回原始 key 本身（而非空字符串），便于调试