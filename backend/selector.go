// Package backend provides a Provider-Selector pattern for Astra web framework.
// It maps runtime environments (dev / prod / staging / test) to backend
// implementations (memory / sql-redis / ...), mirroring the GMS project's
// ProviderSelector.
//
// # Typical usage
//
//	// 1. Create a selector (default: dev→"memory", prod→"sql-redis")
//	s := backend.New("my-svc")
//
//	// 2. Register implementations
//	s.Register("memory", repos.NewInMemory())
//	s.Register("sql-redis", repos.NewSQL(dsn))
//
//	// 3. Select by environment at startup
//	repo := s.MustSelect(cfg.Mode) // cfg.Mode = "dev" → "memory" impl
//
// # With explicit override (e.g. test containers)
//
//	// Force a specific backend regardless of environment
//	s := backend.New("my-svc", backend.WithBackend("sql-redis"))
//	repo := s.MustSelect("dev") // still returns "sql-redis"
//
// # Integration with boot.Service
//
//	svc := boot.New("order-svc",
//	    boot.WithConfigPath("config.yaml"),
//	    boot.WithBackend("sql-redis"),       // explicit backend
//	    boot.WithBackendMapping(myMapping),  // custom env→backend map
//	)
package backend

import (
	"fmt"
	"sort"
	"sync"
)

// DefaultBackendMapping defines the recommended env→backend strategy:
//   - dev / test  → "memory"    (lightweight, zero dependencies, fast restart)
//   - prod / staging → "sql-redis" (production-grade with external stores)
//
// These reflect the GMS pattern: in-memory for local development/testing and
// SQL+Redis for production.  Override via WithMapping or SetMapping.
var DefaultBackendMapping = map[string]string{
	"dev":     "memory",
	"test":    "memory",
	"prod":    "sql-redis",
	"staging": "sql-redis",
}

// BackendSelector maps runtime environments to registered backend
// implementations, following the Provider-Selector injection pattern.
//
//  1. Create with New("name", opts...)
//  2. Register backend implementations by name
//  3. At startup, Select(env) returns the implementation for the runtime env
//
// Thread-safe for concurrent reads after initial setup (RWMutex).
type BackendSelector struct {
	mu       sync.RWMutex
	name     string            // identifier, used in error messages
	backends map[string]any    // name → implementation
	mapping  map[string]string // env → backend name
	explicit string            // non-empty = force this backend, ignore env
}

// Option configures a BackendSelector.
type Option func(*BackendSelector)

// WithMapping sets a custom env→backend mapping.
// Keys are environment names (dev, prod, test, staging);
// Values are backend names that should be registered separately.
// Merges with defaults (same key overwrites).
func WithMapping(mapping map[string]string) Option {
	return func(s *BackendSelector) {
		for k, v := range mapping {
			s.mapping[k] = v
		}
	}
}

// WithBackend forces a specific backend name regardless of environment.
// When set, Select ignores the env argument and always returns the forced
// backend.  Equivalent to GMS's explicit backend configuration field.
func WithBackend(name string) Option {
	return func(s *BackendSelector) {
		s.explicit = name
	}
}

// New creates a BackendSelector with the default env→backend mapping
// (dev/test→"memory", prod/staging→"sql-redis") and no registered
// implementations.  Call Register to populate implementations before Select.
//
//	name: identifier for error messages (e.g. "order-svc", "auth")
func New(name string, opts ...Option) *BackendSelector {
	s := &BackendSelector{
		name:     name,
		backends: make(map[string]any),
		mapping:  make(map[string]string, len(DefaultBackendMapping)),
	}
	for k, v := range DefaultBackendMapping {
		s.mapping[k] = v
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Register stores a named backend implementation.
// Panics if a backend with the same name is already registered
// (use MustRegister to overwrite silently).
func (s *BackendSelector) Register(name string, impl any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.backends[name]; exists {
		panic(fmt.Sprintf("backend: %q already registered in selector %q", name, s.name))
	}
	s.backends[name] = impl
}

// MustRegister registers a backend, silently overwriting any existing
// registration with the same name.
func (s *BackendSelector) MustRegister(name string, impl any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends[name] = impl
}

// Select returns the backend implementation for the given environment.
//
// Resolution order:
//  1. Explicit override (set via WithBackend or Force) — always wins
//  2. Environment mapping (e.g. "dev" → "memory")
//
// Returns (impl, true) if found, or (nil, false) if the environment has no
// mapping or the mapped backend name hasn't been registered.
func (s *BackendSelector) Select(env string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Explicit override wins
	backendName := s.explicit
	if backendName == "" {
		// 2. Look up by env
		var ok bool
		backendName, ok = s.mapping[env]
		if !ok {
			return nil, false
		}
	}
	impl, ok := s.backends[backendName]
	return impl, ok
}

// MustSelect returns the backend implementation for the given environment.
// Panics if no implementation is found.
func (s *BackendSelector) MustSelect(env string) any {
	impl, ok := s.Select(env)
	if !ok {
		envStr := env
		if s.explicit != "" {
			envStr = fmt.Sprintf("%s (forced backend: %q)", env, s.explicit)
		}
		panic(fmt.Sprintf("backend: no implementation for env %q in selector %q "+
			"(registered: %v, mapping: %v, explicit: %q)",
			envStr, s.name, s.sortedBackends(), s.mapping, s.explicit))
	}
	return impl
}

// Force sets an explicit backend override at runtime.  Thread-safe.
// After Force("memory"), Select ignores the env argument and always returns
// the "memory" implementation.
//
// Pass an empty string to clear the override and revert to env-based selection.
func (s *BackendSelector) Force(backend string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.explicit = backend
}

// SetMapping sets or overrides the backend for a specific environment.
// Thread-safe.
//
//	s.SetMapping("dev", "sql-redis")     // use sql-redis in dev environments
//	s.SetMapping("ci", "memory")         // add a custom CI env
func (s *BackendSelector) SetMapping(env, backend string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mapping[env] = backend
}

// Available returns all registered backend names (unsorted).
func (s *BackendSelector) Available() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.backends))
	for name := range s.backends {
		names = append(names, name)
	}
	return names
}

// HasBackend reports whether the given backend name is registered.
func (s *BackendSelector) HasBackend(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.backends[name]
	return ok
}

// Name returns the selector identifier (set via New).
func (s *BackendSelector) Name() string { return s.name }

// Mapping returns a copy of the current env→backend mapping.
func (s *BackendSelector) Mapping() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.mapping))
	for k, v := range s.mapping {
		out[k] = v
	}
	return out
}

// Explicit returns the current forced backend name and whether one is set.
func (s *BackendSelector) Explicit() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.explicit, s.explicit != ""
}

// Count returns the number of registered backends.
func (s *BackendSelector) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.backends)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (s *BackendSelector) sortedBackends() []string {
	names := make([]string, 0, len(s.backends))
	for name := range s.backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
