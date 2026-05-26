// Package lua provides embedded Lua script execution via gopher-lua and
// Redis-backed Lua execution via go-redis EVALSHA/EVAL.
package lua

import (
	"fmt"
	"os"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// Mode controls how Lua states are managed across registered scripts.
type Mode int

const (
	// ModeIsolated gives each registered script its own *lua.LState.
	// Scripts cannot see each other's globals; maximum isolation.
	// Concurrent calls to different scripts are allowed in parallel.
	ModeIsolated Mode = iota

	// ModeShared merges all registered scripts into a single *lua.LState.
	// Functions defined by one script are visible to all others.
	// All calls are serialized via a single mutex.
	ModeShared
)

// Option configures an Engine.
type Option func(*Engine)

// WithMode sets the execution mode (default: ModeIsolated).
func WithMode(m Mode) Option {
	return func(e *Engine) { e.mode = m }
}

type script struct {
	state *lua.LState // non-nil in ModeIsolated
	mu    sync.Mutex  // per-script lock; allows different scripts to run concurrently
	code  string
}

// Engine manages multiple named Lua scripts.
type Engine struct {
	mode    Mode
	scripts map[string]*script
	shared  *lua.LState  // non-nil in ModeShared
	mu      sync.RWMutex // guards scripts map and shared state
}

// New creates a new Engine with the given options.
func New(opts ...Option) *Engine {
	e := &Engine{
		mode:    ModeIsolated,
		scripts: make(map[string]*script),
	}
	for _, o := range opts {
		o(e)
	}
	if e.mode == ModeShared {
		e.shared = lua.NewState()
	}
	return e
}

// Register loads a Lua script from file and registers it under name.
func (e *Engine) Register(name, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("lua: read file %q: %w", file, err)
	}
	return e.RegisterString(name, string(data))
}

// RegisterString registers an inline Lua script under name.
//
// In ModeIsolated, a new *lua.LState is created and the code is executed
// so that all function definitions become globals of that isolated state.
//
// In ModeShared, the code is executed in the shared state, merging its
// function definitions with those from other scripts.
func (e *Engine) RegisterString(name, code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch e.mode {
	case ModeIsolated:
		L := lua.NewState()
		if err := L.DoString(code); err != nil {
			L.Close()
			return fmt.Errorf("lua: compile %q: %w", name, err)
		}
		e.scripts[name] = &script{state: L, code: code}

	case ModeShared:
		if err := e.shared.DoString(code); err != nil {
			return fmt.Errorf("lua: compile %q: %w", name, err)
		}
		e.scripts[name] = &script{code: code}
	}
	return nil
}

// Call invokes the Lua function fn defined in the script registered as name,
// passing args as arguments. Supported Go arg types: string, int, int64,
// float64, bool, []any, map[string]any.
//
// Return values are mapped back to Go: LString→string, LNumber→float64,
// LBool→bool, LNil→nil, LTable→[]any (array) or map[string]any (hash).
//
// In ModeIsolated, only the target script's mutex is held, so calls to
// different scripts may proceed concurrently.
// In ModeShared, the engine-level write lock is held for the duration,
// serializing all calls.
func (e *Engine) Call(name, fn string, args ...any) ([]any, error) {
	// Look up the script under a read lock.
	e.mu.RLock()
	sc, ok := e.scripts[name]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("lua: script %q not registered", name)
	}

	// Convert Go arguments to Lua values.
	var L *lua.LState
	var unlock func()

	switch e.mode {
	case ModeIsolated:
		sc.mu.Lock()
		L = sc.state
		unlock = sc.mu.Unlock
	case ModeShared:
		e.mu.Lock()
		L = e.shared
		unlock = e.mu.Unlock
	}
	defer unlock()

	largs := make([]lua.LValue, len(args))
	for i, a := range args {
		largs[i] = toLua(L, a)
	}

	// Capture stack depth before call so we know how many values were returned.
	top := L.GetTop()

	if err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal(fn),
		NRet:    lua.MultRet,
		Protect: true,
	}, largs...); err != nil {
		return nil, fmt.Errorf("lua: call %q.%s: %w", name, fn, err)
	}

	// Collect return values pushed onto the stack after the call.
	nret := L.GetTop() - top
	results := make([]any, nret)
	for i := 0; i < nret; i++ {
		results[i] = fromLua(L.Get(top + 1 + i))
	}
	L.Pop(nret)
	return results, nil
}

// Close releases all Lua states. Call only after all other operations have
// completed; Close is not safe to call concurrently with other methods.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.mode == ModeShared {
		if e.shared != nil {
			e.shared.Close()
			e.shared = nil
		}
		return
	}
	for _, sc := range e.scripts {
		if sc.state != nil {
			sc.state.Close()
			sc.state = nil
		}
	}
}

// toLua converts a Go value to the corresponding Lua value.
func toLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case string:
		return lua.LString(x)
	case int:
		return lua.LNumber(x)
	case int64:
		return lua.LNumber(x)
	case float64:
		return lua.LNumber(x)
	case bool:
		if x {
			return lua.LTrue
		}
		return lua.LFalse
	case nil:
		return lua.LNil
	case []any:
		t := L.NewTable()
		for i, item := range x {
			L.RawSetInt(t, i+1, toLua(L, item))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, val := range x {
			t.RawSetString(k, toLua(L, val))
		}
		return t
	default:
		return lua.LString(fmt.Sprintf("%v", x))
	}
}

// fromLua converts a Lua value to the corresponding Go value.
func fromLua(v lua.LValue) any {
	switch x := v.(type) {
	case lua.LString:
		return string(x)
	case lua.LNumber:
		return float64(x)
	case lua.LBool:
		return bool(x)
	case *lua.LNilType:
		return nil
	case *lua.LTable:
		// Detect arrays: if all keys are sequential integers starting at 1.
		length := x.Len()
		if length > 0 {
			arr := make([]any, length)
			for i := 1; i <= length; i++ {
				arr[i-1] = fromLua(x.RawGetInt(i))
			}
			return arr
		}
		// Fall back to map for non-sequential or string-keyed tables.
		m := make(map[string]any)
		x.ForEach(func(k, val lua.LValue) {
			m[k.String()] = fromLua(val)
		})
		return m
	default:
		return v.String()
	}
}
