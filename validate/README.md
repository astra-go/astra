# Validate — Data Validation

Structural and field validation based on `go-playground/validator/v10`, with custom validation rules.

## Features

- **Struct Tag Validation**: Declare validation rules via `validate` tag; no manual if-else
- **Built-in Custom Rules**: `mobile` (China mobile), `password` (password strength), `username`, `no_html`, `not_blank`
- **Alias System**: Register validation aliases via `WithAlias`
- **Instance API**: `Validator` instance supports concurrent safety
- **Error Mapping**: `Errors.Map()` returns `{field: message}` format error mapping

## Quick Start

### Basic Usage

```go
import "github.com/astra-go/astra/validate"

type RegisterReq struct {
    Username string `json:"username" validate:"required,username"`
    Email    string `json:"email"    validate:"required,email"`
    Password string `json:"password" validate:"required,password"`
    Mobile   string `json:"mobile"   validate:"omitempty,mobile"`
    Age      int    `json:"age"      validate:"required,gte=18,lte=120"`
    Role     string `json:"role"     validate:"required,oneof=admin user guest"`
}

var req RegisterReq
c.Bind(&req)

if err := validate.Struct(&req); err != nil {
    var verrs validate.Errors
    if errors.As(err, &verrs) {
        c.JSON(400, astra.Map{"errors": verrs.Map()})
    }
    return
}
```

### Custom Alias

```go
v := validate.New(
    validate.WithAlias("strongpw", "required,min=8,max=64,password"),
)
v.Struct(&req)
```

### Single Field Validation

```go
err := validate.Var(value, "required,email")
```

## Common Validation Tags

| Tag | Description | Example |
|-----|-------------|---------|
| `required` | Cannot be zero value | `validate:"required"` |
| `omitempty` | Skip validation on zero value | `validate:"omitempty,email"` |
| `email` | Must be email format | `validate:"email"` |
| `min=5` | Min value (number/string length) | `validate:"min=5"` |
| `max=100` | Max value | `validate:"max=100"` |
| `gte=18` | Greater than or equal | `validate:"gte=18"` |
| `lte=120` | Less than or equal | `validate:"lte=120"` |
| `eq=abc` | Must equal | `validate:"eq=abc"` |
| `ne=abc` | Must not equal | `validate:"ne=abc"` |
| `oneof=admin user` | Must be one of | `validate:"oneof=admin user guest"` |
| `len=10` | Length must equal | `validate:"len=11"` |
| `uuid` | Must be UUID format | `validate:"uuid"` |
| `url` | Must be URL | `validate:"url"` |
| `ip` | Must be IP address | `validate:"ip"` |
| `datetime=2006-01-02` | Datetime format | `validate:"datetime=2006-01-02"` |

### Astra Custom Rules

| Tag | Description | Example |
|-----|-------------|---------|
| `mobile` | China mobile number (1[3-9] 11 digits) | `validate:"mobile"` |
| `password` | Password strength (upper+lower+num+special, ≥8 chars) | `validate:"password"` |
| `username` | Username (a-zA-Z0-9_, 3-32 chars) | `validate:"username"` |
| `no_html` | Cannot contain HTML tags | `validate:"no_html"` |
| `not_blank` | Cannot be empty or whitespace | `validate:"not_blank"` |

## API

### validate.Struct

```go
func Struct(s any) error
```

Validates entire struct. Returns `*Errors` or `nil`.

### validate.Var

```go
func Var(v any, tag string) error
```

Validates single field value.

### Errors Type

```go
type Errors []ValidationError // Slice containing all failures

// Single error
type ValidationError struct {
    Field   string // Field name
    Message string // Error message
}

// Returns {field: message} mapping
func (e Errors) Map() map[string]string
```

## Complete Example

```go
package main

import (
    "fmt"
    "github.com/astra-go/astra/validate"
)

type User struct {
    Name     string `validate:"required,not_blank"`
    Email    string `validate:"required,email"`
    Password string `validate:"required,password"`
    Mobile   string `validate:"omitempty,mobile"`
    Age      int    `validate:"gte=0,lte=150"`
}

func main() {
    u := User{
        Name:     "Alice",
        Email:    "alice@example.com",
        Password: "Pass@1234",
        Mobile:   "13800138000",
        Age:      25,
    }

    err := validate.Struct(&u)
    if err != nil {
        var verrs validate.Errors
        if ok := errors.As(err, &verrs); ok {
            for _, e := range verrs {
                fmt.Printf("Field %s: %s\n", e.Field, e.Message)
            }
        }
        return
    }
    fmt.Println("Validation passed")
}
```

## Module Dependencies

- `github.com/go-playground/validator/v10` — Validation engine

## Notes

- `required` for strings checks "non-empty" (not empty string); empty string fails; use `not_blank` if blank is allowed but whitespace-only is not
- `omitempty` skips entire validation when field is zero value, not just that tag
- Password validation rule: ≥8 chars, contains upper/lower case letters, numbers, and special characters
