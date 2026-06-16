# Rule — Expression Rule Engine

Type-safe expression rule engine based on `github.com/expr-lang/expr`, "closure entry" design prevents injection.

## Features

- **Type Safety**: Expressions check variable types at compile time; type mismatch causes compile failure directly
- **Closure Environment**: Expressions can only access predefined variables and methods, cannot access arbitrary code
- **Generic Return**: `RunBool`, `RunInt`, `RunFloat64` and other type-safe execution functions
- **Custom Functions**: Register custom Go functions to expressions via `Engine.WithFunc`
- **Compiled Caching**: `Program` compiles once and can be executed repeatedly

## Quick Start

```go
import "github.com/astra-go/astra/rule"

type OrderEnv struct {
    Amount float64
    UserVIP bool
}

// Compile (execute once at startup)
prog := rule.MustCompile(`Amount > 1000 && UserVIP`, OrderEnv{})

// Execute (can call multiple times at runtime)
ok, _ := rule.RunBool(prog, OrderEnv{Amount: 1500, UserVIP: true}) // true
ok, _ = rule.RunBool(prog, OrderEnv{Amount: 500, UserVIP: true})   // false
```

## API

### Compile / MustCompile

```go
// Returns error
prog, err := rule.Compile(`Amount > 100`, OrderEnv{})

// panic on error
prog := rule.MustCompile(`Amount > 100`, OrderEnv{})
```

### Run Functions (Type-Safe)

```go
rule.RunBool(prog, env)    (bool, error)
rule.RunInt(prog, env)     (int, error)
rule.RunInt64(prog, env)   (int64, error)
rule.RunFloat64(prog, env) (float64, error)
rule.RunAny(prog, env)     (any, error)
```

### Engine (Advanced)

```go
engine := rule.NewEngine().
    WithFunc("upper", func(p ...any) (any, error) {
        return strings.ToUpper(p[0].(string)), nil
    }, new(func(string) string))

prog, _ := engine.Compile(`upper(Name) == "ALICE"`, UserEnv{})
```

### Option

```go
// Force expression to return bool (error on type mismatch)
rule.AsBool()

// Allow undefined variables in expression (not recommended)
rule.AllowUndefined()
```

## Config

### Custom Function Registration

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

## Complete Example

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
    // Permission check
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

    // Age filter
    ageProg := rule.MustCompile(`Age >= 18 && Age <= 60`, UserEnv{})
    for _, u := range users {
        ok, _ := rule.RunBool(ageProg, u)
        fmt.Printf("Age=%d → eligible=%t\n", u.Age, ok)
    }
}
```

## Module Dependencies

- `github.com/expr-lang/expr` — Expression evaluation engine

## Notes

- `MustCompile` panics on expression syntax error; suitable for startup validation
- Custom function parameter types must be explicitly specified, otherwise compile fails
- For floating point comparisons in expressions, use range checks rather than exact equality (e.g., `Amount > 99.99` instead of `Amount == 100`)
