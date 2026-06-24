package di

import "fmt"

// ─── ProviderSet: grouped registrations for environment-aware DI ──────────────

// ProviderSet groups one or more provider registrations under an optional
// environment tag.  This mirrors the GMS Provider-Selector pattern where
// each microservice defines separate dependency graphs for dev, prod, and
// test environments and selects the right one at startup.
//
// Usage:
//
//	// Define per-environment provider sets
//	devSet := di.NewSet("dev",
//	    func(c *di.Container) error {
//	        return di.Provide[*UserRepo](c, func(_ *di.Container) (*UserRepo, error) {
//	            return rep.NewMemoryUserRepo(), nil
//	        })
//	    },
//	    func(c *di.Container) error {
//	        return di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
//	            repo := di.MustInvoke[*UserRepo](c)
//	            return service.NewUserService(repo), nil
//	        })
//	    },
//	)
//
//	prodSet := di.NewSet("prod",
//	    func(c *di.Container) error {
//	        return di.Provide[*UserRepo](c, func(_ *di.Container) (*UserRepo, error) {
//	            return rep.NewSqlUserRepo(dsn, maxOpen), nil
//	        })
//	    },
//	    ...
//	)
//
//	// At startup, select by APP_MODE:
//	container := di.New()
//	err := di.RegisterSets(container, "dev", devSet, prodSet)
//	// or without error handling:
//	di.MustRegisterSets(container, os.Getenv("APP_MODE"), devSet, prodSet)
//
// ProviderSet methods are safe for reuse across containers: each Apply call
// re-runs the provider functions on the target container.
type ProviderSet struct {
	env       string
	providers []func(*Container) error
}

// NewSet creates a ProviderSet tagged for the given environment.
// Use "" or "default" for providers that should run in any environment.
func NewSet(env string, providers ...func(*Container) error) *ProviderSet {
	return &ProviderSet{env: env, providers: providers}
}

// NewDefaultSet creates a ProviderSet with no environment tag.
// It always applies when passed to RegisterSets.
func NewDefaultSet(providers ...func(*Container) error) *ProviderSet {
	return &ProviderSet{env: "default", providers: providers}
}

// Env returns the environment tag of this set.
func (s *ProviderSet) Env() string { return s.env }

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
