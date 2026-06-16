# Rule — 表达式规则引擎

基于 `github.com/expr-lang/expr` 的类型安全表达式规则引擎，"闭包入口"设计防止注入。

## 特性

- **类型安全**：表达式在编译时检查变量类型；类型不匹配直接导致编译失败
- **闭包环境**：表达式只能访问预定义变量和方法，不能访问任意代码
- **泛型返回**：`RunBool`、`RunInt`、`RunFloat64` 等类型安全执行函数
- **自定义函数**：通过 `Engine.WithFunc` 向表达式注册自定义 Go 函数
- **编译缓存**：`Program` 编译一次可重复执行

## 快速开始

```go
import "github.com/astra-go/astra/rule"

type OrderEnv struct {
    Amount float64
    UserVIP bool
}

// 编译（启动时执行一次）
prog := rule.MustCompile(`Amount > 1000 && UserVIP`, OrderEnv{})

// 执行（运行时可多次调用）
ok, _ := rule.RunBool(prog, OrderEnv{Amount: 1500, UserVIP: true}) // true
ok, _ = rule.RunBool(prog, OrderEnv{Amount: 500, UserVIP: true})   // false
```

## API

### Compile / MustCompile

```go
// 返回错误
prog, err := rule.Compile(`Amount > 100`, OrderEnv{})

// 错误时 panic
prog := rule.MustCompile(`Amount > 100`, OrderEnv{})
```

### Run 函数（类型安全）

```go
rule.RunBool(prog, env)    (bool, error)
rule.RunInt(prog, env)     (int, error)
rule.RunInt64(prog, env)   (int64, error)
rule.RunFloat64(prog, env) (float64, error)
rule.RunAny(prog, env)     (any, error)
```

### Engine（高级）

```go
engine := rule.NewEngine().
    WithFunc("upper", func(p ...any) (any, error) {
        return strings.ToUpper(p[0].(string)), nil
    }, new(func(string) string))

prog, _ := engine.Compile(`upper(Name) == "ALICE"`, UserEnv{})
```

### Option

```go
// 强制表达式返回 bool（类型不匹配时报错）
rule.AsBool()

// 允许表达式中的未定义变量（不推荐）
rule.AllowUndefined()
```

## 配置

### 自定义函数注册

```go
import "github.com/astra-go/astra/rule"

engine := rule.NewEngine().
    WithFunc("in", func(p ...any) (any, error) {
        val := p[0]
        list := p[1:]
        for _, v := range list {
            if v == val { return true, nil }
        }
        return false, nil
    }, new(func(string, ...string) bool))

prog, _ := engine.Compile(`in(UserRole, "admin", "superadmin")`, Env{})
```

## 完整示例

```go
package main

import (
    "fmt"
    "github.com/astra-go/astra/rule"
)

type UserEnv struct {
    Age      int
    Role     string
    Approved bool
}

func main() {
    // 权限检查
    permProg := rule.MustCompile(
        `Role == "admin" || Role == "superadmin"`,
        UserEnv{},
    )

    users := []UserEnv{
        {Age: 25, Role: "user", Approved: true},
        {Age: 30, Role: "admin", Approved: true},
        {Age: 35, Role: "superadmin", Approved: false},
    }

    for _, u := range users {
        ok, _ := rule.RunBool(permProg, u)
        fmt.Printf("Role=%s, Approved=%t → can_access=%t\n", u.Role, u.Approved, ok)
    }

    // 年龄过滤
    ageProg := rule.MustCompile(`Age >= 18 && Age <= 60`, UserEnv{})
    for _, u := range users {
        ok, _ := rule.RunBool(ageProg, u)
        fmt.Printf("Age=%d → eligible=%t\n", u.Age, ok)
    }
}
```

## 模块依赖

- `github.com/expr-lang/expr` — 表达式求值引擎

## 注意事项

- `MustCompile` 在表达式语法错误时 panic；适合启动时校验
- 自定义函数参数类型必须显式指定，否则编译失败
- 表达式中的浮点数比较建议使用范围检查而非精确相等（如 `Amount > 99.99` 而非 `Amount == 100`）