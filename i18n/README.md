# i18n — Internationalization

Zero-dependency thread-safe multi-language messages package with built-in languages and runtime extension.

## Features

- **Built-in Languages**: en, zh/zh-CN, zh-TW/zh-HK, ja, ko, fr, de, es, pt/pt-BR, ru, ar
- **Runtime Registration/Extension**: Can register new languages or override existing translations at startup
- **Context-Aware**: `i18n.T(c, key)` auto-detects current request's language
- **Middleware Integration**: `i18n.Middleware()` auto-sets language from Accept-Language header
- **Go Template Support**: Message format follows `fmt.Sprintf` spec, supports `%s`, `%d`, etc.

## Quick Start

### Initialization

```go
import "github.com/astra-go/astra/i18n"

// Register custom messages (override or add language)
i18n.Register("zh", i18n.Messages{
    "validate.required": "%s cannot be blank",
    "http.404":          "Resource not found",
    "common.welcome":    "Welcome to %s",
})
```

### Middleware Style (Recommended)

```go
app.Use(i18n.Middleware())

func handler(c *astra.Ctx) error {
    msg := i18n.T(c, "common.success")
    return c.JSON(200, astra.Map{"message": msg})
}
```

### Direct Call

```go
// Manual language setting
i18n.SetLanguage("ja")
msg := i18n.T(context.Background(), "validate.required")
// Template args
msg := i18n.Format(context.Background(), "common.welcome", "Astra")
```

## API

### i18n.T

```go
func T(c contract.Context, key string) string

// Use Background when not dependent on request context
func T(ctx context.Context, key string) string
```

Gets message template for current language and formats with `fmt.Sprintf`.

### i18n.Format

```go
func Format(ctx context.Context, key string, args ...any) string
```

Passes template args directly, returns formatted string.

### i18n.Register

```go
func Register(locale string, msgs Messages)
```

Registers or overrides a language's message dictionary:

```go
i18n.Register("fr", i18n.Messages{
    "hello": "Bonjour, %s!",
})
```

### i18n.Extend

```go
func Extend(locale string, msgs Messages)
```

Appends messages to existing language (existing keys not overwritten):

```go
i18n.Extend("zh", i18n.Messages{
    "app.brand": "MyApp",
})
```

### i18n.SetLanguage

```go
func SetLanguage(locale string)
```

Sets package-level default language (all subsequent `T` calls use it).

### Bundle (Advanced)

```go
// Create independent multi-language bundle
bundle := i18n.New()
bundle.Register("de", i18n.Messages{"hello": "Hallo"})
bundle.Register("fr", i18n.Messages{"hello": "Bonjour"})
bundle.SetFallback("en")

// Get translator from bundle
t := bundle.Tr("de")
msg := t.T("hello")
```

## Complete Example

```go
package main

import (
    "fmt"

    "github.com/astra-go/astra/i18n"
)

func main() {
    // Register custom translations
    i18n.Register("zh", i18n.Messages{
        "validate.required": "%s cannot be blank",
        "order.success":     "Order %s created successfully",
    })

    // English
    i18n.SetLanguage("en")
    fmt.Println(i18n.Format(nil, "validate.required", "Email"))
    // Output: Email cannot be blank

    // Chinese
    i18n.SetLanguage("zh")
    fmt.Println(i18n.Format(nil, "validate.required", "邮箱"))
    // Output: 邮箱 cannot be blank
}
```

## Module Dependencies

No external dependencies.

## Notes

- `i18n.T(nil, key)` uses package-level default language
- In middleware style, language auto-parses from request's `Accept-Language` header (takes highest-weight item)
- `i18n.T(c, key)` prioritizes Context language, then package-level default
- When key not found, returns the original key itself (not empty string), convenient for debugging
