# Validate — 数据校验

基于 `go-playground/validator/v10` 的结构和字段校验，支持自定义校验规则。

## 特性

- **Struct Tag 校验**：通过 `validate` tag 声明校验规则，无需手动 if-else
- **内置自定义规则**：`mobile`（中国手机号）、`password`（密码强度）、`username`、`no_html`、`not_blank`
- **别名系统**：通过 `WithAlias` 注册校验别名
- **实例 API**：`Validator` 实例支持并发安全
- **错误映射**：`Errors.Map()` 返回 `{field: message}` 格式的错误映射

## 快速开始

### 基础用法

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

### 自定义别名

```go
v := validate.New(
    validate.WithAlias("strongpw", "required,min=8,max=64,password"),
)
v.Struct(&req)
```

### 单字段校验

```go
err := validate.Var(value, "required,email")
```

## 常用校验 Tag

| Tag | 说明 | 示例 |
|-----|-------------|---------|
| `required` | 不能为零值 | `validate:"required"` |
| `omitempty` | 为零值时跳过校验 | `validate:"omitempty,email"` |
| `email` | 必须为邮箱格式 | `validate:"email"` |
| `min=5` | 最小值（数字/字符串长度） | `validate:"min=5"` |
| `max=100` | 最大值 | `validate:"max=100"` |
| `gte=18` | 大于等于 | `validate:"gte=18"` |
| `lte=120` | 小于等于 | `validate:"lte=120"` |
| `eq=abc` | 必须等于 | `validate:"eq=abc"` |
| `ne=abc` | 不能等于 | `validate:"ne=abc"` |
| `oneof=admin user` | 必须为其中之一 | `validate:"oneof=admin user guest"` |
| `len=10` | 长度必须等于 | `validate:"len=11"` |
| `uuid` | 必须为 UUID 格式 | `validate:"uuid"` |
| `url` | 必须为 URL | `validate:"url"` |
| `ip` | 必须为 IP 地址 | `validate:"ip"` |
| `datetime=2006-01-02` | 日期时间格式 | `validate:"datetime=2006-01-02"` |

### Astra 自定义规则

| Tag | 说明 | 示例 |
|-----|-------------|---------|
| `mobile` | 中国手机号（1[3-9] 11位） | `validate:"mobile"` |
| `password` | 密码强度（大写+小写+数字+特殊字符，≥8位） | `validate:"password"` |
| `username` | 用户名（a-zA-Z0-9_，3-32位） | `validate:"username"` |
| `no_html` | 不能包含 HTML 标签 | `validate:"no_html"` |
| `not_blank` | 不能为空或纯空白 | `validate:"not_blank"` |

## API

### validate.Struct

```go
func Struct(s any) error
```

校验整个结构体。返回 `*Errors` 或 `nil`。

### validate.Var

```go
func Var(v any, tag string) error
```

校验单个字段值。

### Errors 类型

```go
type Errors []ValidationError // 包含所有失败的切片

// 单个错误
type ValidationError struct {
    Field   string // 字段名
    Message string // 错误消息
}

// 返回 {field: message} 映射
func (e Errors) Map() map[string]string
```

## 完整示例

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

## 模块依赖

- `github.com/go-playground/validator/v10` — 校验引擎

## 注意事项

- `required` 对字符串检查"非空"（非空字符串）；空字符串会失败；如果允许空白但不接受空使用 `not_blank`
- `omitempty` 在字段为零值时跳过整个校验，不只是该 tag
- 密码校验规则：≥8位，包含大小写字母、数字和特殊字符