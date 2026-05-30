// Package di provides a lightweight, type-safe dependency injection container
// for Astra applications.
//
// The container uses Go generics so all registrations and resolutions are
// checked at compile time; there are no string-typed bindings or interface{}
// casts in application code.
//
// All instances are singletons by default: the factory function is called at
// most once per key (even under concurrent access via sync.Once).
//
// # Quick start
//
//	c := di.New()
//
//	// Register a database connection
//	di.Provide[*sql.DB](c, func(c *di.Container) (*sql.DB, error) {
//	    return sql.Open("postgres", os.Getenv("DATABASE_URL"))
//	})
//
//	// Register a service that depends on *sql.DB
//	di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
//	    db, err := di.Invoke[*sql.DB](c)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return NewUserService(db), nil
//	})
//
//	// Bind lifecycle to Astra app
//	app := astra.New()
//	c.BindApp(app)
//
//	// Resolve in a handler
//	app.GET("/users", func(ctx *astra.Ctx) error {
//	    svc := di.MustInvoke[*UserService](c)
//	    return ctx.JSON(200, svc.List(ctx.Request.Context()))
//	})
//
// # Named instances
//
// When you need multiple implementations of the same interface, use
// ProvideNamed / InvokeNamed:
//
//	di.ProvideNamed[Cache](c, "local",  func(c *di.Container) (Cache, error) { ... })
//	di.ProvideNamed[Cache](c, "remote", func(c *di.Container) (Cache, error) { ... })
//
//	local  := di.MustInvokeNamed[Cache](c, "local")
//	remote := di.MustInvokeNamed[Cache](c, "remote")
//
// # Lifecycle hooks
//
// Register start and stop hooks directly on the container or add them when
// providing a service:
//
//	di.Provide[*Worker](c, func(c *di.Container) (*Worker, error) {
//	    w := NewWorker()
//	    c.OnStart(func(ctx context.Context) error { return w.Start(ctx) })
//	    c.OnStop(func(ctx context.Context) error { return w.Stop(ctx) })
//	    return w, nil
//	})
//
// Call c.BindApp(app) once to wire the container's Start/Stop into Astra's
// graceful-shutdown lifecycle.
package di

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/astra-go/astra"
)

// ─── sentinel errors ──────────────────────────────────────────────────────────

// ErrNotFound is returned when no provider has been registered for the
// requested type (and optional name).
var ErrNotFound = errors.New("di: provider not found")

// ErrDuplicate is returned when a provider for the same type (and name) is
// registered more than once.
var ErrDuplicate = errors.New("di: provider already registered")

// ErrCyclicDependency is the panic value when a circular dependency is
// detected during resolution.  The panic carries a wrapped error so callers
// can use errors.Is(recover().(error), di.ErrCyclicDependency).
var ErrCyclicDependency = errors.New("di: circular dependency detected")

// ErrMaxDepthExceeded is the panic value when the resolution depth exceeds
// the configured maximum (default 32).
var ErrMaxDepthExceeded = errors.New("di: maximum resolution depth exceeded")

// ─── cycle-detection helpers ─────────────────────────────────────────────────

// resolvingStack is the per-goroutine ordered list of type keys currently
// being resolved.  A key that appears in the stack is "in-flight"; seeing it
// a second time means a cycle.
type resolvingStack []typeKey

// goroutineID extracts the current goroutine's numeric ID from the runtime
// stack header "goroutine NNN [".  Used only at resolution time (startup),
// never on the hot request path.
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Header is always "goroutine NNN ["; skip the 10-byte "goroutine " prefix.
	if n < 11 {
		return 0
	}
	var id uint64
	for _, c := range buf[10:n] {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}

// cyclePath builds a human-readable cycle string, e.g.
// "*UserService → *DB → *UserService".  Only the sub-path starting at the
// first occurrence of repeated is included, so transitive callers are omitted.
func cyclePath(stack resolvingStack, repeated typeKey) string {
	start := 0
	for i, k := range stack {
		if k == repeated {
			start = i
			break
		}
	}
	var sb strings.Builder
	for _, k := range stack[start:] {
		sb.WriteString(k.String())
		sb.WriteString(" → ")
	}
	sb.WriteString(repeated.String())
	return sb.String()
}

// depthPath builds a human-readable dependency chain for depth-exceeded errors,
// e.g. "*ServiceA → *ServiceB → *ServiceC → *ServiceD".
func depthPath(stack resolvingStack, current typeKey) string {
	var sb strings.Builder
	for i, k := range stack {
		sb.WriteString(k.String())
		if i < len(stack)-1 {
			sb.WriteString(" → ")
		}
	}
	if len(stack) > 0 {
		sb.WriteString(" → ")
	}
	sb.WriteString(current.String())
	return sb.String()
}

// ─── internal key ─────────────────────────────────────────────────────────────

// typeKey identifies a provider by its resolved Go type and an optional name.
// The anonymous key (name == "") is the default; named keys enable multiple
// implementations of the same type.
type typeKey struct {
	typ  reflect.Type
	name string
}

func (k typeKey) String() string {
	if k.name == "" {
		return k.typ.String()
	}
	return fmt.Sprintf("%s(%q)", k.typ.String(), k.name)
}

// ─── provider entry ───────────────────────────────────────────────────────────

// entry holds one registered factory together with its singleton result.
// sync.Once guarantees the factory runs at most once, even when multiple
// goroutines call Invoke concurrently.
type entry struct {
	key   typeKey                    // for cycle-detection error messages
	build func(c *Container) (any, error)
	once  sync.Once
	value any
	err   error
}

// resolve calls build at most once and returns the cached result on subsequent
// calls.  Before delegating to sync.Once it checks the goroutine-local
// resolution stack stored in the container; if the key is already present the
// call chain forms a cycle and this function panics with ErrCyclicDependency.
func (e *entry) resolve(c *Container) (any, error) {
	gid := goroutineID()

	// Retrieve or lazily create the resolution stack for this goroutine.
	stackVal, _ := c.goroutineStacks.LoadOrStore(gid, &resolvingStack{})
	stack := stackVal.(*resolvingStack)

	// 1. Cycle check (MUST BE FIRST - highest priority)
	// If our key is already on the stack the factory chain has re-entered
	// its own resolution on the same goroutine.
	for _, k := range *stack {
		if k == e.key {
			panic(fmt.Errorf("%w: %s", ErrCyclicDependency, cyclePath(*stack, e.key)))
		}
	}

	// 2. Depth check (second priority)
	c.mu.RLock()
	maxDepth := c.maxDepth
	c.mu.RUnlock()

	if maxDepth > 0 && len(*stack) >= maxDepth {
		panic(fmt.Errorf("%w (limit: %d): %s",
			ErrMaxDepthExceeded, maxDepth, depthPath(*stack, e.key)))
	}

	// 3. Push our key; defer the pop so it runs even if the factory panics.
	*stack = append(*stack, e.key)
	defer func() {
		*stack = (*stack)[:len(*stack)-1]
		if len(*stack) == 0 {
			c.goroutineStacks.Delete(gid)
		}
		// Ensure cleanup even on panic
		if r := recover(); r != nil {
			panic(r)
		}
	}()

	// 4. Execute factory
	e.once.Do(func() {
		e.value, e.err = e.build(c)
	})
	return e.value, e.err
}

// ─── lifecycle hook ───────────────────────────────────────────────────────────

type hookEntry struct {
	fn func(context.Context) error
}

// ─── Container ────────────────────────────────────────────────────────────────

// Container is a thread-safe dependency injection container.
// Instances are singletons: factories are called at most once per type key.
//
// Use the package-level generic functions (Provide, Invoke, …) to interact
// with a Container; do not embed or copy it after first use.
type Container struct {
	mu         sync.RWMutex
	providers  map[typeKey]*entry
	startHooks []hookEntry
	stopHooks  []hookEntry
	goroutineStacks sync.Map
	maxDepth int
}

// New creates an empty Container with default maxDepth of 32.
func New() *Container {
	return &Container{
		providers: make(map[typeKey]*entry),
		maxDepth:  32,
	}
}

// WithMaxDepth sets the maximum resolution depth limit.
// depth=0 means unlimited (only cycle detection applies).
// Default is 32.
// Returns the container for method chaining.
func (c *Container) WithMaxDepth(depth int) *Container {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxDepth = depth
	return c
}

// ─── registration helpers ─────────────────────────────────────────────────────

func typeOf[T any]() reflect.Type {
	// (*T)(nil) is a typed nil pointer; .Elem() unwraps to the actual type T.
	// This works for concrete types, pointers, interfaces, and generics.
	return reflect.TypeOf((*T)(nil)).Elem()
}

func register[T any](c *Container, name string, factory func(*Container) (any, error)) error {
	k := typeKey{typ: typeOf[T](), name: name}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.providers[k]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, k)
	}
	c.providers[k] = &entry{key: k, build: factory}
	return nil
}

// ─── public registration API ─────────────────────────────────────────────────

// Provide registers a factory for type T.
// The factory receives the container so it can resolve its own dependencies.
// The factory is called at most once (singleton).
// Returns ErrDuplicate if T has already been registered.
func Provide[T any](c *Container, factory func(*Container) (T, error)) error {
	return register[T](c, "", func(c *Container) (any, error) {
		return factory(c)
	})
}

// ProvideNamed registers a factory for type T under the given name.
// Use this when multiple implementations of the same type are needed.
func ProvideNamed[T any](c *Container, name string, factory func(*Container) (T, error)) error {
	if name == "" {
		return fmt.Errorf("di: name must not be empty; use Provide for the anonymous key")
	}
	return register[T](c, name, func(c *Container) (any, error) {
		return factory(c)
	})
}

// ProvideValue registers a pre-built singleton value for type T.
// Equivalent to Provide[T] with a factory that always returns value.
func ProvideValue[T any](c *Container, value T) error {
	return register[T](c, "", func(*Container) (any, error) {
		return value, nil
	})
}

// ProvideValueNamed registers a pre-built singleton value for type T under name.
func ProvideValueNamed[T any](c *Container, name string, value T) error {
	if name == "" {
		return fmt.Errorf("di: name must not be empty; use ProvideValue for the anonymous key")
	}
	return register[T](c, name, func(*Container) (any, error) {
		return value, nil
	})
}

// ─── public resolution API ────────────────────────────────────────────────────

// Invoke resolves the singleton instance of type T.
// Returns ErrNotFound if no provider has been registered.
func Invoke[T any](c *Container) (T, error) {
	return invoke[T](c, "")
}

// InvokeNamed resolves the singleton instance of type T registered under name.
// Returns ErrNotFound if no provider has been registered for that name.
func InvokeNamed[T any](c *Container, name string) (T, error) {
	return invoke[T](c, name)
}

// MustInvoke resolves type T and panics if no provider is registered or if
// the factory returns an error.
// Intended for use at application startup where a missing dependency is a
// programmer error, not a recoverable runtime condition.
func MustInvoke[T any](c *Container) T {
	v, err := Invoke[T](c)
	if err != nil {
		panic(err)
	}
	return v
}

// MustInvokeNamed resolves named type T and panics on error.
func MustInvokeNamed[T any](c *Container, name string) T {
	v, err := InvokeNamed[T](c, name)
	if err != nil {
		panic(err)
	}
	return v
}

func invoke[T any](c *Container, name string) (T, error) {
	var zero T
	k := typeKey{typ: typeOf[T](), name: name}

	c.mu.RLock()
	e, ok := c.providers[k]
	c.mu.RUnlock()

	if !ok {
		return zero, fmt.Errorf("%w: %s", ErrNotFound, k)
	}

	raw, err := e.resolve(c)
	if err != nil {
		return zero, fmt.Errorf("di: factory for %s: %w", k, err)
	}

	v, ok := raw.(T)
	if !ok {
		// Should not happen in practice: the factory's return type is T.
		return zero, fmt.Errorf("di: internal type mismatch for %s: got %T", k, raw)
	}
	return v, nil
}

// ─── introspection ────────────────────────────────────────────────────────────

// Has reports whether a provider has been registered for type T.
func Has[T any](c *Container) bool {
	k := typeKey{typ: typeOf[T](), name: ""}
	c.mu.RLock()
	_, ok := c.providers[k]
	c.mu.RUnlock()
	return ok
}

// HasNamed reports whether a named provider has been registered for type T.
func HasNamed[T any](c *Container, name string) bool {
	k := typeKey{typ: typeOf[T](), name: name}
	c.mu.RLock()
	_, ok := c.providers[k]
	c.mu.RUnlock()
	return ok
}

// Len returns the total number of registered providers.
func (c *Container) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.providers)
}

// ─── lifecycle ────────────────────────────────────────────────────────────────

// OnStart registers a hook to run when Start is called.
// Hooks run in registration order.
//
// This method is safe to call from inside a Provide factory so that a service
// can register its own lifecycle hooks at construction time.
func (c *Container) OnStart(fn func(context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startHooks = append(c.startHooks, hookEntry{fn: fn})
}

// OnStop registers a hook to run when Stop is called.
// Hooks run in reverse registration order (LIFO), which is the natural
// teardown order when services depend on each other.
func (c *Container) OnStop(fn func(context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopHooks = append(c.stopHooks, hookEntry{fn: fn})
}

// Start runs all registered start hooks in registration order.
// It stops and returns the first error encountered.
func (c *Container) Start(ctx context.Context) error {
	c.mu.RLock()
	hooks := make([]hookEntry, len(c.startHooks))
	copy(hooks, c.startHooks)
	c.mu.RUnlock()

	for _, h := range hooks {
		if err := h.fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stop runs all registered stop hooks in reverse registration order (LIFO).
// Unlike Start, Stop runs every hook even when one returns an error, and
// returns the first error seen (subsequent errors are discarded).
func (c *Container) Stop(ctx context.Context) error {
	c.mu.RLock()
	hooks := make([]hookEntry, len(c.stopHooks))
	copy(hooks, c.stopHooks)
	c.mu.RUnlock()

	var first error
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := hooks[i].fn(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// BindApp wires the container's lifecycle into an Astra application.
// c.Start is called before the HTTP server starts accepting connections;
// c.Stop is called during graceful shutdown.
//
// Call BindApp once, before app.Run.
func (c *Container) BindApp(app *astra.App) {
	app.OnStart(c.Start)
	app.OnStop(c.Stop)
}
