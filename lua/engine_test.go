package lua_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/astra-go/astra/lua"
)

// ─── ModeIsolated ─────────────────────────────────────────────────────────────

func TestEngine_IsolatedMode_FunctionsIsolated(t *testing.T) {
	eng := lua.New() // default: ModeIsolated
	defer eng.Close()

	// Two scripts each define a function called "value" that returns different results.
	if err := eng.RegisterString("a", `function value() return "from-a" end`); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := eng.RegisterString("b", `function value() return "from-b" end`); err != nil {
		t.Fatalf("register b: %v", err)
	}

	ra, err := eng.Call("a", "value")
	if err != nil {
		t.Fatalf("call a.value: %v", err)
	}
	rb, err := eng.Call("b", "value")
	if err != nil {
		t.Fatalf("call b.value: %v", err)
	}

	if ra[0] != "from-a" {
		t.Errorf("a.value: want 'from-a', got %v", ra[0])
	}
	if rb[0] != "from-b" {
		t.Errorf("b.value: want 'from-b', got %v", rb[0])
	}
}

// ─── ModeShared ───────────────────────────────────────────────────────────────

func TestEngine_SharedMode_FunctionsVisible(t *testing.T) {
	eng := lua.New(lua.WithMode(lua.ModeShared))
	defer eng.Close()

	// Script "a" defines helper(); script "b" calls it.
	if err := eng.RegisterString("a", `function helper() return "shared" end`); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := eng.RegisterString("b", `function use_helper() return helper() end`); err != nil {
		t.Fatalf("register b: %v", err)
	}

	res, err := eng.Call("b", "use_helper")
	if err != nil {
		t.Fatalf("call b.use_helper: %v", err)
	}
	if res[0] != "shared" {
		t.Errorf("want 'shared', got %v", res[0])
	}
}

// ─── Argument and return types ────────────────────────────────────────────────

func TestEngine_Call_StringArg(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	eng.RegisterString("s", `function echo(x) return x end`)
	res, err := eng.Call("s", "echo", "hello")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res[0] != "hello" {
		t.Errorf("want 'hello', got %v", res[0])
	}
}

func TestEngine_Call_NumberArg(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	eng.RegisterString("n", `function double(x) return x * 2 end`)
	res, err := eng.Call("n", "double", float64(7))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res[0] != float64(14) {
		t.Errorf("want 14, got %v", res[0])
	}
}

func TestEngine_Call_BoolArg(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	eng.RegisterString("b", `function negate(x) return not x end`)
	res, err := eng.Call("b", "negate", true)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res[0] != false {
		t.Errorf("want false, got %v", res[0])
	}
}

func TestEngine_Call_MultipleReturnValues(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	eng.RegisterString("m", `function pair() return "x", 42 end`)
	res, err := eng.Call("m", "pair")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 return values, got %d", len(res))
	}
	if res[0] != "x" || res[1] != float64(42) {
		t.Errorf("want ('x', 42), got (%v, %v)", res[0], res[1])
	}
}

// ─── Error handling ───────────────────────────────────────────────────────────

func TestEngine_RegisterString_SyntaxError(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	err := eng.RegisterString("bad", `function broken( -- syntax error`)
	if err == nil {
		t.Error("expected error for invalid Lua syntax")
	}
}

func TestEngine_Call_UnknownScript_ReturnsError(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	_, err := eng.Call("ghost", "fn")
	if err == nil {
		t.Error("expected error for unregistered script name")
	}
}

func TestEngine_Call_UnknownFunction_ReturnsError(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	eng.RegisterString("s", `function exists() return 1 end`)
	_, err := eng.Call("s", "does_not_exist")
	if err == nil {
		t.Error("expected error when calling undefined Lua function")
	}
}

// ─── Register from file ───────────────────────────────────────────────────────

func TestEngine_Register_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "greet.lua")
	if err := os.WriteFile(path, []byte(`function greet(name) return "Hi " .. name end`), 0o600); err != nil {
		t.Fatalf("write temp lua file: %v", err)
	}

	eng := lua.New()
	defer eng.Close()

	if err := eng.Register("greet", path); err != nil {
		t.Fatalf("Register: %v", err)
	}

	res, err := eng.Call("greet", "greet", "Alice")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res[0] != "Hi Alice" {
		t.Errorf("want 'Hi Alice', got %v", res[0])
	}
}

func TestEngine_Register_FileNotFound_ReturnsError(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	if err := eng.Register("x", "/no/such/file.lua"); err == nil {
		t.Error("expected error for missing file")
	}
}

// ─── Close ────────────────────────────────────────────────────────────────────

func TestEngine_Close_DoesNotPanic(t *testing.T) {
	eng := lua.New()
	eng.RegisterString("x", `function f() return 1 end`)
	eng.Close() // must not panic
}

func TestEngine_Close_Shared_DoesNotPanic(t *testing.T) {
	eng := lua.New(lua.WithMode(lua.ModeShared))
	eng.RegisterString("x", `function f() return 1 end`)
	eng.Close()
}

// ─── Concurrent safety ────────────────────────────────────────────────────────

func TestEngine_ConcurrentCalls(t *testing.T) {
	eng := lua.New()
	defer eng.Close()

	// Register 5 independent scripts.
	scripts := []string{"s0", "s1", "s2", "s3", "s4"}
	for _, name := range scripts {
		code := `function add(a, b) return a + b end`
		if err := eng.RegisterString(name, code); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		name := scripts[i%len(scripts)]
		go func(n string) {
			defer wg.Done()
			res, err := eng.Call(n, "add", float64(1), float64(2))
			if err != nil {
				t.Errorf("concurrent Call %s: %v", n, err)
				return
			}
			if res[0] != float64(3) {
				t.Errorf("concurrent Call %s: want 3, got %v", n, res[0])
			}
		}(name)
	}
	wg.Wait()
}

func TestEngine_ConcurrentCalls_Shared(t *testing.T) {
	eng := lua.New(lua.WithMode(lua.ModeShared))
	defer eng.Close()

	if err := eng.RegisterString("s", `function add(a, b) return a + b end`); err != nil {
		t.Fatalf("register: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := eng.Call("s", "add", float64(5), float64(6))
			if err != nil {
				t.Errorf("shared concurrent Call: %v", err)
				return
			}
			if res[0] != float64(11) {
				t.Errorf("shared concurrent Call: want 11, got %v", res[0])
			}
		}()
	}
	wg.Wait()
}
