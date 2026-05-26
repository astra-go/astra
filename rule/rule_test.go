package rule_test

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/astra-go/astra/rule"
)

// ─── Test environments ────────────────────────────────────────────────────────

type OrderEnv struct {
	Amount   float64
	Quantity int
	UserVIP  bool
	Status   string
}

type UserEnv struct {
	Name  string
	Age   int
	Email string
	Role  string
}

// Methods on an env struct become callable functions in expressions.
type MathEnv struct {
	X float64
	Y float64
}

func (e MathEnv) Sum() float64     { return e.X + e.Y }
func (e MathEnv) Product() float64 { return e.X * e.Y }

// ─── Compile ──────────────────────────────────────────────────────────────────

func TestCompile_ValidBoolExpr(t *testing.T) {
	prog, err := rule.Compile(`Amount > 100`, OrderEnv{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if prog == nil {
		t.Fatal("Compile: returned nil program")
	}
}

func TestCompile_SyntaxError(t *testing.T) {
	_, err := rule.Compile(`Amount >>< 100`, OrderEnv{})
	if err == nil {
		t.Fatal("Compile(syntax error): expected error")
	}
}

func TestCompile_UndefinedField_ReturnsError(t *testing.T) {
	// "Closed entry" guarantee: referencing a non-existent field is a compile error.
	_, err := rule.Compile(`NonExistentField > 100`, OrderEnv{})
	if err == nil {
		t.Fatal("Compile(undefined field): expected error — closed entry violated")
	}
}

func TestCompile_AsBool_TypeMismatch(t *testing.T) {
	// AsBool() rejects an expression that returns a number.
	_, err := rule.Compile(`Amount * 2`, OrderEnv{}, rule.AsBool())
	if err == nil {
		t.Fatal("Compile(AsBool on numeric expr): expected type error")
	}
}

func TestCompile_AsBool_Valid(t *testing.T) {
	_, err := rule.Compile(`Amount > 0 && UserVIP`, OrderEnv{}, rule.AsBool())
	if err != nil {
		t.Fatalf("Compile(AsBool valid): %v", err)
	}
}

func TestMustCompile_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustCompile: expected panic for invalid expression")
		}
	}()
	rule.MustCompile(`!!!bad expression`, OrderEnv{})
}

func TestProgram_String(t *testing.T) {
	src := `Amount > 500`
	prog := rule.MustCompile(src, OrderEnv{})
	if prog.String() != src {
		t.Errorf("Program.String: want %q, got %q", src, prog.String())
	}
}

// ─── RunBool ──────────────────────────────────────────────────────────────────

func TestRunBool_AmountThreshold(t *testing.T) {
	prog := rule.MustCompile(`Amount > 1000 && UserVIP`, OrderEnv{}, rule.AsBool())

	cases := []struct {
		env  OrderEnv
		want bool
	}{
		{OrderEnv{Amount: 1500, UserVIP: true}, true},
		{OrderEnv{Amount: 1500, UserVIP: false}, false},
		{OrderEnv{Amount: 500, UserVIP: true}, false},
		{OrderEnv{Amount: 0, UserVIP: false}, false},
	}
	for _, c := range cases {
		got, err := rule.RunBool(prog, c.env)
		if err != nil {
			t.Fatalf("RunBool: %v", err)
		}
		if got != c.want {
			t.Errorf("RunBool(%+v): want %v, got %v", c.env, c.want, got)
		}
	}
}

func TestRunBool_StringComparison(t *testing.T) {
	prog := rule.MustCompile(`Status == "pending" || Status == "new"`, OrderEnv{}, rule.AsBool())

	if ok, _ := rule.RunBool(prog, OrderEnv{Status: "pending"}); !ok {
		t.Error("RunBool(pending): expected true")
	}
	if ok, _ := rule.RunBool(prog, OrderEnv{Status: "shipped"}); ok {
		t.Error("RunBool(shipped): expected false")
	}
}

func TestRunBool_StringBuiltins(t *testing.T) {
	// expr-lang/expr provides built-in functions: contains, startsWith, endsWith, etc.
	prog := rule.MustCompile(`Email contains "@"`, UserEnv{}, rule.AsBool())
	if ok, _ := rule.RunBool(prog, UserEnv{Email: "alice@example.com"}); !ok {
		t.Error("RunBool(contains @): expected true")
	}
	if ok, _ := rule.RunBool(prog, UserEnv{Email: "notanemail"}); ok {
		t.Error("RunBool(no @): expected false")
	}
}

func TestRunBool_TernaryExpression(t *testing.T) {
	prog := rule.MustCompile(`Age >= 18 ? true : false`, UserEnv{}, rule.AsBool())
	if ok, _ := rule.RunBool(prog, UserEnv{Age: 20}); !ok {
		t.Error("Ternary(adult): expected true")
	}
	if ok, _ := rule.RunBool(prog, UserEnv{Age: 15}); ok {
		t.Error("Ternary(minor): expected false")
	}
}

// ─── Run (any result) ─────────────────────────────────────────────────────────

func TestRun_ArithmeticResult(t *testing.T) {
	prog := rule.MustCompile(`Amount * 0.9`, OrderEnv{})
	out, err := rule.Run(prog, OrderEnv{Amount: 200.0})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := out.(float64)
	if got != 180.0 {
		t.Errorf("Run(Amount*0.9): want 180, got %v", got)
	}
}

func TestRun_StringConcatenation(t *testing.T) {
	prog := rule.MustCompile(`"Hello, " + Name + "!"`, UserEnv{})
	out, err := rule.Run(prog, UserEnv{Name: "Alice"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.(string) != "Hello, Alice!" {
		t.Errorf("Run(concat): want 'Hello, Alice!', got %v", out)
	}
}

func TestRunFloat64_Success(t *testing.T) {
	prog := rule.MustCompile(`X + Y`, MathEnv{})
	got, err := rule.RunFloat64(prog, MathEnv{X: 3.5, Y: 1.5})
	if err != nil || got != 5.0 {
		t.Errorf("RunFloat64: want 5.0, got %v, err %v", got, err)
	}
}

// ─── Env methods ──────────────────────────────────────────────────────────────

func TestRun_EnvMethod(t *testing.T) {
	// Methods on the env struct are callable in expressions.
	prog := rule.MustCompile(`Sum() > 10`, MathEnv{})
	if ok, _ := rule.RunBool(prog, MathEnv{X: 6, Y: 7}); !ok {
		t.Error("Sum()>10: expected true for 6+7=13")
	}
}

func TestRun_EnvMethod_Product(t *testing.T) {
	prog := rule.MustCompile(`Product() == 12.0`, MathEnv{}, rule.AsBool())
	if ok, _ := rule.RunBool(prog, MathEnv{X: 3, Y: 4}); !ok {
		t.Error("Product()==12: expected true for 3*4=12")
	}
}

// ─── Engine ───────────────────────────────────────────────────────────────────

func TestEngine_WithFunc_Abs(t *testing.T) {
	engine := rule.NewEngine().WithFunc("abs",
		func(p ...any) (any, error) {
			f, ok := p[0].(float64)
			if !ok {
				return nil, fmt.Errorf("abs: expected float64, got %T", p[0])
			}
			return math.Abs(f), nil
		},
		new(func(float64) float64),
	)

	prog, err := engine.Compile(`abs(X) > 5`, MathEnv{})
	if err != nil {
		t.Fatalf("Engine.Compile: %v", err)
	}

	if ok, _ := engine.RunBool(prog, MathEnv{X: -10}); !ok {
		t.Error("abs(-10)>5: expected true")
	}
	if ok, _ := engine.RunBool(prog, MathEnv{X: -3}); ok {
		t.Error("abs(-3)>5: expected false")
	}
}

func TestEngine_WithFunc_StringUpper(t *testing.T) {
	engine := rule.NewEngine().WithFunc("upper",
		func(p ...any) (any, error) {
			return strings.ToUpper(p[0].(string)), nil
		},
		new(func(string) string),
	)

	prog := engine.MustCompile(`upper(Name) == "ALICE"`, UserEnv{})
	if ok, _ := engine.RunBool(prog, UserEnv{Name: "alice"}); !ok {
		t.Error("upper(alice)==ALICE: expected true")
	}
}

func TestEngine_MultipleChainedFuncs(t *testing.T) {
	engine := rule.NewEngine().
		WithFunc("double",
			func(p ...any) (any, error) { return p[0].(float64) * 2, nil },
			new(func(float64) float64),
		).
		WithFunc("square",
			func(p ...any) (any, error) { f := p[0].(float64); return f * f, nil },
			new(func(float64) float64),
		)

	prog := engine.MustCompile(`double(X) + square(Y) == 10`, MathEnv{}, rule.AsBool())
	// double(2)=4, square(√6)... let's use X=3,Y=2: double(3)=6, square(2)=4 → 10
	if ok, _ := engine.RunBool(prog, MathEnv{X: 3, Y: 2}); !ok {
		t.Error("double(3)+square(2)==10: expected true")
	}
}

func TestEngine_UndefinedFunc_CompileError(t *testing.T) {
	engine := rule.NewEngine() // no functions registered
	_, err := engine.Compile(`mystery(Amount)`, OrderEnv{})
	if err == nil {
		t.Error("Engine.Compile(unknown func): expected error")
	}
}

// ─── Concurrent safety ────────────────────────────────────────────────────────

func TestRun_ConcurrentSafe(t *testing.T) {
	prog := rule.MustCompile(`Amount > 100 && UserVIP`, OrderEnv{}, rule.AsBool())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			env := OrderEnv{Amount: float64(n * 20), UserVIP: n%2 == 0}
			_, err := rule.RunBool(prog, env)
			if err != nil {
				t.Errorf("concurrent RunBool: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

// ─── Practical rule engine scenario ──────────────────────────────────────────

// discountEngine demonstrates a business rule engine pattern:
// rules are compiled once at startup and evaluated per-request.
type OrderRule struct {
	Expr    string
	Discount float64
}

var discountRules []struct {
	prog     *rule.Program
	discount float64
}

func init() {
	engine := rule.NewEngine()
	rules := []OrderRule{
		{`Amount >= 5000 && UserVIP`, 0.2},
		{`Amount >= 2000`, 0.1},
		{`Amount >= 1000`, 0.05},
	}
	for _, r := range rules {
		prog := engine.MustCompile(r.Expr, OrderEnv{}, rule.AsBool())
		discountRules = append(discountRules, struct {
			prog     *rule.Program
			discount float64
		}{prog, r.Discount})
	}
}

func applyDiscount(env OrderEnv) float64 {
	for _, r := range discountRules {
		if ok, _ := rule.RunBool(r.prog, env); ok {
			return r.discount
		}
	}
	return 0
}

func TestDiscountRuleEngine(t *testing.T) {
	cases := []struct {
		env  OrderEnv
		want float64
	}{
		{OrderEnv{Amount: 6000, UserVIP: true}, 0.2},
		{OrderEnv{Amount: 3000, UserVIP: false}, 0.1},
		{OrderEnv{Amount: 1200, UserVIP: false}, 0.05},
		{OrderEnv{Amount: 500, UserVIP: false}, 0},
	}
	for _, c := range cases {
		if got := applyDiscount(c.env); got != c.want {
			t.Errorf("applyDiscount(%+v): want %.2f, got %.2f", c.env, c.want, got)
		}
	}
}
