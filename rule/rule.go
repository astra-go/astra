// Package rule provides a type-safe expression rule engine built on expr-lang/expr.
//
// It wraps the [expr-lang/expr] library with a "closed entry" design:
// every expression is compiled against a typed Go struct that defines the only
// variables and methods accessible inside the expression. This prevents
// injection of arbitrary code and guarantees compile-time type safety.
//
// # Quick start
//
//	type OrderEnv struct {
//	    Amount float64
//	    UserVIP bool
//	}
//
//	// Compile once at startup (fails fast on bad expressions)
//	prog := rule.MustCompile(`Amount > 1000 && UserVIP`, OrderEnv{})
//
//	// Run many times with different data
//	ok, _ := rule.RunBool(prog, OrderEnv{Amount: 1500, UserVIP: true}) // true
//	ok, _ = rule.RunBool(prog, OrderEnv{Amount: 500, UserVIP: true})   // false
//
// # Custom functions via Engine
//
//	engine := rule.NewEngine().
//	    WithFunc("upper", func(p ...any) (any, error) {
//	        return strings.ToUpper(p[0].(string)), nil
//	    }, new(func(string) string))
//
//	prog, _ := engine.Compile(`upper(Name) == "ALICE"`, UserEnv{})
package rule

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// Program is a compiled expression. It is immutable and safe for concurrent use.
// Compile once at startup; call Run or RunBool many times with different envs.
type Program struct {
	inner *vm.Program
	src   string // original expression, for error messages
}

// String returns the original expression source.
func (p *Program) String() string { return p.src }

// Option is an alias for expr.Option, exposed here so callers need not import
// expr-lang/expr directly for common configuration options.
type Option = expr.Option

// ─── Option helpers ───────────────────────────────────────────────────────────
// These are thin wrappers around the matching expr.As* functions.

// AsBool tells the compiler the expression must return a bool.
// Compile returns an error if the expression yields any other type.
func AsBool() Option { return expr.AsBool() }

// AsInt tells the compiler the expression must return an int.
func AsInt() Option { return expr.AsInt() }

// AsInt64 tells the compiler the expression must return an int64.
func AsInt64() Option { return expr.AsInt64() }

// AsFloat64 tells the compiler the expression must return a float64.
func AsFloat64() Option { return expr.AsFloat64() }

// AsAny tells the compiler the expression may return any type.
func AsAny() Option { return expr.AsAny() }

// AllowUndefined allows the expression to reference variables not defined in
// the environment. Disabled by default to enforce "closed entry".
func AllowUndefined() Option { return expr.AllowUndefinedVariables() }

// ─── Package-level compile/run ────────────────────────────────────────────────

// Compile compiles expression against the typed env struct.
//
// env must be a non-nil struct value (not a pointer, not a map). All fields and
// methods of env (and embedded fields) are accessible inside the expression.
// The expression cannot access any Go symbols outside of env — this is the
// "closed entry" guarantee.
//
// Compilation is safe for concurrent use; the returned *Program is immutable.
//
//	type PriceEnv struct{ Price, Threshold float64 }
//	prog, err := rule.Compile(`Price >= Threshold`, PriceEnv{})
func Compile(expression string, env any, opts ...Option) (*Program, error) {
	allOpts := make([]Option, 0, 1+len(opts))
	allOpts = append(allOpts, expr.Env(env))
	allOpts = append(allOpts, opts...)
	p, err := expr.Compile(expression, allOpts...)
	if err != nil {
		return nil, fmt.Errorf("rule: compile %q: %w", expression, err)
	}
	return &Program{inner: p, src: expression}, nil
}

// MustCompile is like Compile but panics on error.
// Use at package-level initialization where a bad expression is a programming error.
//
//	var discountRule = rule.MustCompile(`Amount > 1000`, OrderEnv{}, rule.AsBool())
func MustCompile(expression string, env any, opts ...Option) *Program {
	p, err := Compile(expression, env, opts...)
	if err != nil {
		panic(err)
	}
	return p
}

// Run evaluates prog with the given environment and returns the raw result.
// env must be the same concrete type used when the program was compiled.
//
// Run is safe for concurrent use.
func Run(prog *Program, env any) (any, error) {
	out, err := expr.Run(prog.inner, env)
	if err != nil {
		return nil, fmt.Errorf("rule: run %q: %w", prog.src, err)
	}
	return out, nil
}

// RunBool evaluates prog and returns the result as bool.
// Returns ErrNotBool if the expression produced a non-boolean value.
//
// For guaranteed compile-time type checking, compile with rule.AsBool().
func RunBool(prog *Program, env any) (bool, error) {
	out, err := Run(prog, env)
	if err != nil {
		return false, err
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("rule: RunBool %q: expression returned %T, expected bool", prog.src, out)
	}
	return b, nil
}

// RunInt64 evaluates prog and returns the result as int64.
func RunInt64(prog *Program, env any) (int64, error) {
	out, err := Run(prog, env)
	if err != nil {
		return 0, err
	}
	switch v := out.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	}
	return 0, fmt.Errorf("rule: RunInt64 %q: expression returned %T, expected numeric", prog.src, out)
}

// RunFloat64 evaluates prog and returns the result as float64.
func RunFloat64(prog *Program, env any) (float64, error) {
	out, err := Run(prog, env)
	if err != nil {
		return 0, err
	}
	switch v := out.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	}
	return 0, fmt.Errorf("rule: RunFloat64 %q: expression returned %T, expected numeric", prog.src, out)
}

// ─── Engine ───────────────────────────────────────────────────────────────────

// Engine is a rule compiler pre-loaded with registered custom functions.
// Use it when multiple rules need access to the same helper functions.
//
// Methods are fluent — they return the same *Engine for chaining:
//
//	engine := rule.NewEngine().
//	    WithFunc("abs", func(p ...any) (any, error) {
//	        if f, ok := p[0].(float64); ok { return math.Abs(f), nil }
//	        return nil, fmt.Errorf("abs: expected float64")
//	    }, new(func(float64) float64))
type Engine struct {
	opts []Option
}

// NewEngine creates a new Engine with no registered functions.
func NewEngine() *Engine {
	return &Engine{}
}

// WithFunc registers a named function that expressions can call.
//
// fn must have the signature func(params ...any) (any, error). The variadic
// params slice contains the arguments passed by the expression.
//
// Optional types provide type overloads: each element must be a pointer to a
// function type such as new(func(float64) float64). The compiler uses these to
// infer argument and return types at compile time.
//
//	engine.WithFunc("min2",
//	    func(p ...any) (any, error) {
//	        a, b := p[0].(float64), p[1].(float64)
//	        if a < b { return a, nil }
//	        return b, nil
//	    },
//	    new(func(float64, float64) float64),
//	)
func (e *Engine) WithFunc(name string, fn func(params ...any) (any, error), types ...any) *Engine {
	e.opts = append(e.opts, expr.Function(name, fn, types...))
	return e
}

// Compile compiles an expression using all registered functions.
// The env and opts arguments are identical to the package-level Compile.
func (e *Engine) Compile(expression string, env any, opts ...Option) (*Program, error) {
	allOpts := make([]Option, 0, 1+len(e.opts)+len(opts))
	allOpts = append(allOpts, expr.Env(env))
	allOpts = append(allOpts, e.opts...) // engine-registered functions
	allOpts = append(allOpts, opts...)   // call-site overrides
	p, err := expr.Compile(expression, allOpts...)
	if err != nil {
		return nil, fmt.Errorf("rule: compile %q: %w", expression, err)
	}
	return &Program{inner: p, src: expression}, nil
}

// MustCompile is like Compile but panics on error.
func (e *Engine) MustCompile(expression string, env any, opts ...Option) *Program {
	p, err := e.Compile(expression, env, opts...)
	if err != nil {
		panic(err)
	}
	return p
}

// Run evaluates prog; delegates to the package-level Run.
func (e *Engine) Run(prog *Program, env any) (any, error) { return Run(prog, env) }

// RunBool evaluates prog and returns a bool result.
func (e *Engine) RunBool(prog *Program, env any) (bool, error) { return RunBool(prog, env) }

// RunFloat64 evaluates prog and returns a float64 result.
func (e *Engine) RunFloat64(prog *Program, env any) (float64, error) {
	return RunFloat64(prog, env)
}
