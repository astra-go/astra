package di

import (
	"fmt"
	"reflect"
)

// ─── ProviderSet: grouped registrations for environment-aware DI ──────────────

// ProviderSet groups one or more provider registrations under an optional
// environment tag.  This mirrors the GMS Provider-Selector pattern where
// each microservice defines separate dependency graphs for dev, prod, and
// test environments and selects the right one at startup.
//
// Usage with generic provider functions:
//
//	// Simple providers (no dependencies)
//	devSet := di.NewSet("dev",
//	    di.ProvideFunc(func() (*UserRepo, error) {
//	        return rep.NewMemoryUserRepo(), nil
//	    }),
//	    di.ProvideFunc(func() (*UserService, error) {
//	        return service.NewUserService(), nil
//	    }),
//	)
//
//	// Providers with dependencies (use FactoryFunc)
//	devSet := di.NewSet("dev",
//	    di.FactoryFunc(func(c *di.Container) (*UserService, error) {
//	        repo, _ := di.Invoke[*UserRepo](c)
//	        return service.NewUserService(repo), nil
//	    }),
//	)
//
//	// Mixed usage
//	container := di.New()
//	di.MustRegisterSets(container, "dev", devSet, prodSet)
//
// ProviderSet methods are safe for reuse across containers: each Apply call
// re-runs the provider functions on the target container.
type ProviderSet struct {
	env       string
	providers []func(*Container) error
}

// NewSet creates a ProviderSet tagged for the given environment.
// Use "" or "default" for providers that should run in any environment.
// Accepts both legacy func(*Container) error and generic ProviderFunc/FactoryFunc.
func NewSet(env string, providers ...any) *ProviderSet {
	registrars := make([]func(*Container) error, 0, len(providers))
	for _, p := range providers {
		registrars = append(registrars, toRegistrar(p))
	}
	return &ProviderSet{env: env, providers: registrars}
}

// NewDefaultSet creates a ProviderSet with no environment tag.
// It always applies when passed to RegisterSets.
func NewDefaultSet(providers ...any) *ProviderSet {
	return NewSet("default", providers...)
}

// Env returns the environment tag of this set.
func (s *ProviderSet) Env() string { return s.env }

// ─── Generic Provider Functions ───────────────────────────────────────────────

// ProviderFunc is a generic provider function that does not require Container.
// Use this for providers that create instances without depending on other DI services.
// The type parameter T is the concrete return type; it must match what callers
// will Invoke from the container.
//
// Example — register a singleton repository:
//
//	var _ di.ProviderFunc[*UserRepo] = func() (*UserRepo, error) {
//	    return rep.NewMemoryUserRepo(), nil
//	}
//
//	// In a ProviderSet:
//	devSet := di.NewSet("dev",
//	    di.ProvideFunc(func() (*UserRepo, error) { return rep.NewMemoryUserRepo(), nil }),
//	    di.ProvideFunc(func() (*UserService, error) { return service.NewUserService(), nil }),
//	)
//
// The provider function is called once; the returned instance is registered as
// a singleton. If the function returns a non-nil error the registration aborts
// and the container creation fails.
type ProviderFunc[T any] func() (T, error)

// FactoryFunc is a generic provider factory that receives *Container for dependency injection.
// Use this when you need to resolve other services from the container before
// building the target type. The type parameter T is the concrete return type.
//
// Example — resolve a dependency before constructing the target:
//
//	var _ di.FactoryFunc[*UserService] = func(c *di.Container) (*UserService, error) {
//	    repo, _ := di.Invoke[*UserRepo](c)
//	    return service.NewUserService(repo), nil
//	}
//
//	// In a ProviderSet:
//	container := di.New()
//	di.MustRegisterSets(container, "dev",
//	    di.NewSet("",
//	        di.ProvideFunc(func() (*UserRepo, error) { return rep.NewMemoryUserRepo(), nil }),
//	    ),
//	    di.NewSet("dev",
//	        di.FactoryFunc(func(c *di.Container) (*UserService, error) {
//	            repo, _ := di.Invoke[*UserRepo](c)
//	            return service.NewUserService(repo), nil
//	        }),
//	    ),
//	)
//
// The returned instance is registered as a singleton (built once, cached for all
// subsequent Invocations). Use when the factory needs to resolve dependencies
// from the container to construct the target type.
type FactoryFunc[T any] func(*Container) (T, error)

// toRegistrar converts various provider types to func(*Container) error.
// Supports:
//   - func(*Container) error (legacy form)
//   - ProviderFunc[T] (generic, no container needed)
//   - FactoryFunc[T] (generic, with container)
func toRegistrar(provider any) func(*Container) error {
	// Handle nil
	if provider == nil {
		return func(*Container) error { return nil }
	}

	switch p := provider.(type) {
	case func(*Container) error:
		return p
	}

	// Handle generic provider types using reflection
	v := reflect.ValueOf(provider)
	typ := v.Type()

	if typ.Kind() != reflect.Func {
		panic(fmt.Errorf("di: unsupported provider type %T (expected function)", provider))
	}

	numIn, numOut := typ.NumIn(), typ.NumOut()
	if numOut != 2 {
		panic(fmt.Errorf("di: provider function must return (value, error), got %d returns", numOut))
	}

	// Verify second return is error
	errType := typ.Out(1)
	if !errType.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		panic(fmt.Errorf("di: second return value must be error, got %s", errType))
	}

	if numIn == 0 {
		// ProviderFunc pattern: func() (T, error)
		// Call provider once, then register the pre-built value as a singleton.
		return func(c *Container) error {
			result := v.Call(nil)
			if !result[1].IsNil() {
				return result[1].Interface().(error)
			}
			return registerSingleton(c, typ.Out(0), result[0].Interface())
		}
	} else if numIn == 1 {
		// Verify first argument is *Container
		if typ.In(0) != reflect.TypeOf((*Container)(nil)) {
			panic(fmt.Errorf("di: first argument must be *Container, got %s", typ.In(0)))
		}

		// FactoryFunc pattern: func(*Container) (T, error)
		// Call provider once, then register the pre-built value as a singleton.
		return func(c *Container) error {
			containerVal := reflect.ValueOf(c)
			result := v.Call([]reflect.Value{containerVal})
			if !result[1].IsNil() {
				return result[1].Interface().(error)
			}
			return registerSingleton(c, typ.Out(0), result[0].Interface())
		}
	}

	panic(fmt.Errorf("di: unsupported provider function signature %T (expected func() (T, error) or func(*Container) (T, error))", provider))
}

// registerSingleton registers a pre-built singleton value with the container.
// The value has already been instantiated by the caller; this function only
// performs the duplicate-registration check and stores it.
func registerSingleton(c *Container, targetType reflect.Type, val any) error {
	if val != nil && reflect.TypeOf(val) != targetType {
		return fmt.Errorf("di: provider returned %T, expected %s", val, targetType)
	}
	k := typeKey{typ: targetType, name: ""}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.providers[k]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, k)
	}
	c.providers[k] = &entry{
		key:   k,
		build: func(*Container) (any, error) { return val, nil },
	}
	return nil
}

// Apply registers all providers in this set into the container.
// Stops at the first registration error and returns it.
func (s *ProviderSet) Apply(c *Container) error {
	for _, fn := range s.providers {
		if err := fn(c); err != nil {
			return fmt.Errorf("di: provider set %q: %w", s.env, err)
		}
	}
	return nil
}

// MustApply registers all providers in this set and panics on error.
func (s *ProviderSet) MustApply(c *Container) {
	if err := s.Apply(c); err != nil {
		panic(err)
	}
}

// ─── Registration: select and apply provider sets by environment ──────────────

// RegisterSets selects the matching ProviderSet for the given environment and
// applies it.  Default/un-tagged sets always run first, followed by the
// environment-specific set.
//
// Matching rules:
//   - Sets with env="" or "default" always apply (as base/fallback providers).
//   - The set whose env matches the requested env is applied last, so it can
//     override default registrations.
//   - If no environment-specific set is found, only default sets apply
//     (no error; useful when defaults are self-sufficient for dev).
//
// Returns the first error from any provider function.  Stops at that error.
func RegisterSets(c *Container, env string, sets ...*ProviderSet) error {
	for _, s := range sets {
		if s.env == "" || s.env == "default" {
			if err := s.Apply(c); err != nil {
				return err
			}
		}
	}
	for _, s := range sets {
		if s.env == env {
			if err := s.Apply(c); err != nil {
				return err
			}
		}
	}
	// Note: if no environment-specific set matches env, only
	// default sets apply — which is fine when defaults are self-sufficient.
	return nil
}

// MustRegisterSets calls RegisterSets and panics on error.
func MustRegisterSets(c *Container, env string, sets ...*ProviderSet) {
	if err := RegisterSets(c, env, sets...); err != nil {
		panic(err)
	}
}

// ─── SelectSet: select a single ProviderSet by environment ────────────────────
// Useful when you want to inspect or test a specific set before applying.

// SelectSet returns the first ProviderSet whose environment matches the given
// env, or nil if no match is found.  Default/un-tagged sets are NOT returned
// by SelectSet; use RegisterSets when you need base + env-specific sets.
func SelectSet(env string, sets ...*ProviderSet) *ProviderSet {
	for _, s := range sets {
		if s.env == env {
			return s
		}
	}
	return nil
}
