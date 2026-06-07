// Package module provides a modular application framework for Astra.
//
// A Module encapsulates a self-contained feature domain: it registers routes,
// providers, middleware, health probes, and lifecycle hooks. Modules can declare
// dependencies on other modules, and the framework resolves the startup order
// automatically using topological sort.
//
// # Quick start
//
//	func NewCacheModule() *module.Module {
//	    return module.New("cache").
//	        WithDependsOn("database").
//	        WithProvider(func(c *di.Container) (*RedisClient, error) {
//	            return redis.NewClient(redisOptions), nil
//	        }).
//	        WithHealthProbe("redis", func(ctx context.Context) error {
//	            return redisClient.Ping(ctx)
//	        }).
//	        WithRoute(func(app *astra.App) {
//	            app.GET("/cache/stats", handleStats)
//	        })
//	}
//
//	func main() {
//	    app := astra.New()
//	    r := module.NewRegistry(app)
//	    r.Register(NewDatabaseModule())
//	    r.Register(NewCacheModule())
//	    if err := r.Start(context.Background()); err != nil {
//	        log.Fatal(err)
//	    }
//	    app.Run(":8080")
//	}
package module

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/di"
)

// Phase controls when a module's Start hook runs relative to others.
// Modules with lower Phase values start first.
type Phase int

const (
	PhaseConfig  Phase = 0 // Configuration loading (config clients, vault)
	PhaseInfra   Phase = 1 // Infrastructure (database, cache, message queues)
	PhaseService Phase = 2 // Business services
	PhaseGateway Phase = 3 // HTTP/gRPC routes, middleware, gateway
	PhaseApp     Phase = 4 // Application-level finalization
)

func (p Phase) String() string {
	switch p {
	case PhaseConfig:
		return "config"
	case PhaseInfra:
		return "infra"
	case PhaseService:
		return "service"
	case PhaseGateway:
		return "gateway"
	case PhaseApp:
		return "app"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

// Module is a self-contained application component.
//
// It bundles routes, DI providers, health probes, middleware, and lifecycle
// hooks behind a named, typed interface. Modules declare dependencies on
// other modules by name; the Registry resolves ordering automatically.
type Module struct {
	Name        string
	Description string
	Phase       Phase
	DependsOn   []string

	// Registration callbacks — called in order by Registry.Start.
	Providers   []ProviderFunc
	HealthProbes []HealthProbeFunc
	Routes      []RouteFunc
	Middlewares []MiddlewareFunc
	OnStart     []func(context.Context) error
	OnStop      []func(context.Context) error
}

// ProviderFunc registers a DI provider into the container.
type ProviderFunc func(c *di.Container)

// HealthProbeFunc registers a health probe into the health module.
type HealthProbeFunc func(r HealthRegistrar)

// HealthRegistrar is a minimal interface for registering health probes.
type HealthRegistrar interface {
	RegisterProbe(name string, probe func(ctx context.Context) error)
}

// RouteFunc registers routes on the Astra app.
type RouteFunc func(app *astra.App)

// MiddlewareFunc registers middleware on the Astra app.
type MiddlewareFunc func(app *astra.App)

// New creates a Module with the given name.
func New(name string) *Module {
	return &Module{
		Name:  name,
		Phase: PhaseService, // sensible default
	}
}

// Desc sets a human-readable description.
func (m *Module) Desc(d string) *Module {
	m.Description = d
	return m
}

// SetPhase sets the startup phase.
func (m *Module) SetPhase(p Phase) *Module {
	m.Phase = p
	return m
}

// WithDependsOn declares module dependencies.
func (m *Module) WithDependsOn(names ...string) *Module {
	m.DependsOn = append(m.DependsOn, names...)
	return m
}

// WithProvider adds a DI provider factory.
func (m *Module) WithProvider(fn ProviderFunc) *Module {
	m.Providers = append(m.Providers, fn)
	return m
}

// WithHealthProbe adds a health probe registrar.
func (m *Module) WithHealthProbe(fn HealthProbeFunc) *Module {
	m.HealthProbes = append(m.HealthProbes, fn)
	return m
}

// WithRoute adds a route registrar.
func (m *Module) WithRoute(fn RouteFunc) *Module {
	m.Routes = append(m.Routes, fn)
	return m
}

// WithMiddleware adds a middleware registrar.
func (m *Module) WithMiddleware(fn MiddlewareFunc) *Module {
	m.Middlewares = append(m.Middlewares, fn)
	return m
}

// WithStartHook adds a start lifecycle hook.
func (m *Module) WithStartHook(fn func(context.Context) error) *Module {
	m.OnStart = append(m.OnStart, fn)
	return m
}

// WithStopHook adds a stop lifecycle hook.
func (m *Module) WithStopHook(fn func(context.Context) error) *Module {
	m.OnStop = append(m.OnStop, fn)
	return m
}

// ─── Registry ────────────────────────────────────────────────────────────────

// Registry manages module registration, dependency resolution, and startup.
type Registry struct {
	app     *astra.App
	container *di.Container
	mu      sync.RWMutex
	modules map[string]*Module
	// optional hooks
	healthRegistrar HealthRegistrar
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithContainer sets the DI container. If nil, a new one is created.
func WithContainer(c *di.Container) RegistryOption {
	return func(r *Registry) { r.container = c }
}

// WithHealthRegistrar sets the health registrar for module probes.
func WithHealthRegistrar(hr HealthRegistrar) RegistryOption {
	return func(r *Registry) { r.healthRegistrar = hr }
}

// NewRegistry creates a module registry bound to the given Astra app.
func NewRegistry(app *astra.App, opts ...RegistryOption) *Registry {
	r := &Registry{
		app:     app,
		modules: make(map[string]*Module),
	}
	for _, o := range opts {
		o(r)
	}
	if r.container == nil {
		r.container = di.New()
	}
	return r
}

// Register adds a module. Panics on duplicate names.
func (r *Registry) Register(m *Module) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.modules[m.Name]; ok {
		panic(fmt.Sprintf("module: duplicate module name %q", m.Name))
	}
	r.modules[m.Name] = m
}

// MustRegister calls Register and panics on error.
func (r *Registry) MustRegister(m *Module) {
	r.Register(m)
}

// Lookup returns a registered module by name, or nil.
func (r *Registry) Lookup(name string) *Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modules[name]
}

// List returns all registered module names, sorted.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.modules))
	for n := range r.modules {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Container returns the DI container.
func (r *Registry) Container() *di.Container {
	return r.container
}

// Start resolves dependencies and starts all modules in order.
// It calls: providers → health probes → middlewares → routes → onStart hooks.
func (r *Registry) Start(ctx context.Context) error {
	r.mu.RLock()
	modules := make(map[string]*Module, len(r.modules))
	for k, v := range r.modules {
		modules[k] = v
	}
	r.mu.RUnlock()

	order, err := resolveOrder(modules)
	if err != nil {
		return fmt.Errorf("module: %w", err)
	}

	// Phase 1: Register all DI providers (in dependency order)
	for _, mod := range order {
		for _, p := range mod.Providers {
			p(r.container)
		}
	}

	// Phase 2: Register health probes
	if r.healthRegistrar != nil {
		for _, mod := range order {
			for _, hp := range mod.HealthProbes {
				hp(r.healthRegistrar)
			}
		}
	}

	// Phase 3: Register middleware
	for _, mod := range order {
		for _, mw := range mod.Middlewares {
			mw(r.app)
		}
	}

	// Phase 4: Register routes
	for _, mod := range order {
		for _, route := range mod.Routes {
			route(r.app)
		}
	}

	// Phase 5: Run start hooks
	for _, mod := range order {
		for _, hook := range mod.OnStart {
			if err := hook(ctx); err != nil {
				return fmt.Errorf("module %q start: %w", mod.Name, err)
			}
		}
	}

	return nil
}

// Stop runs all module stop hooks in reverse order.
func (r *Registry) Stop(ctx context.Context) {
	r.mu.RLock()
	modules := make(map[string]*Module, len(r.modules))
	for k, v := range r.modules {
		modules[k] = v
	}
	r.mu.RUnlock()

	order, _ := resolveOrder(modules)
	for i := len(order) - 1; i >= 0; i-- {
		mod := order[i]
		for j := len(mod.OnStop) - 1; j >= 0; j-- {
			if err := mod.OnStop[j](ctx); err != nil {
				// Log but don't abort — same as Lifecycle behavior
				_ = err // could wire to slog
			}
		}
	}
}

// GetProxy creates a typed inter-module proxy with optional circuit breaking.
//
// The proxy resolves the service from DI on each call, so dynamically
// registered modules are always reachable.
//
//	proxy := registry.GetProxy[*UserService]("service")
//	err := proxy.Call(ctx, 5*time.Second, func(svc *UserService) error {
//	    svc.DoWork(ctx)
//	    return nil
//	})
func GetProxy[T any](r *Registry, opts ...ProxyOption) *Proxy[T] {
	cfg := proxyConfig{
		retries: 0,
	}
	for _, o := range opts {
		o(&cfg)
	}

	p := &Proxy[T]{
		resolve: func() (T, error) {
			return di.Invoke[T](r.container)
		},
		retries: cfg.retries,
	}

	if cfg.breakerEnabled {
		p.circuit = newCircuitState(cfg.breakerThreshold, cfg.breakerTimeout)
	}

	return p
}

// BindApp wires registry lifecycle into the Astra app.
// Call this after registering all modules but before app.Run.
func (r *Registry) BindApp() {
	r.app.OnStart(r.Start)
	r.app.OnStop(func(ctx context.Context) error {
		r.Stop(ctx)
		return nil
	})
	r.container.BindApp(r.app)
}

// ─── Dependency Resolution ────────────────────────────────────────────────────

// resolveOrder performs topological sort on modules based on DependsOn.
// Modules are sorted first by Phase, then by dependency edges.
func resolveOrder(modules map[string]*Module) ([]*Module, error) {
	// Kahn's algorithm with phase priority

	// Collect all names
	names := make([]string, 0, len(modules))
	for n := range modules {
		names = append(names, n)
	}

	// Build in-degree map
	inDegree := make(map[string]int)
	deps := make(map[string][]string) // name → who depends on it
	for _, n := range names {
		inDegree[n] = 0
	}

	for _, n := range names {
		m := modules[n]
		for _, dep := range m.DependsOn {
			if _, ok := modules[dep]; !ok {
				return nil, fmt.Errorf("module %q depends on unknown module %q", n, dep)
			}
			deps[dep] = append(deps[dep], n)
			inDegree[n]++
		}
	}

	// Seed with zero in-degree modules, sorted by phase then name
	var queue []*Module
	for _, n := range names {
		if inDegree[n] == 0 {
			queue = append(queue, modules[n])
		}
	}
	sortModules(queue)

	var order []*Module
	for len(queue) > 0 {
		// Pop first
		m := queue[0]
		queue = queue[1:]
		order = append(order, m)

		// Reduce in-degree of dependents
		for _, dep := range deps[m.Name] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, modules[dep])
			}
		}
		sortModules(queue) // re-sort to maintain phase ordering
	}

	if len(order) != len(modules) {
		// Cycle detected
		remaining := make([]string, 0)
		for n := range modules {
			found := false
			for _, m := range order {
				if m.Name == n {
					found = true
					break
				}
			}
			if !found {
				remaining = append(remaining, n)
			}
		}
		return nil, fmt.Errorf("circular dependency detected among modules: %s",
			strings.Join(remaining, ", "))
	}

	return order, nil
}

func sortModules(mods []*Module) {
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Phase != mods[j].Phase {
			return mods[i].Phase < mods[j].Phase
		}
		return mods[i].Name < mods[j].Name
	})
}
