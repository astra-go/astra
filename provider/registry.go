// Package provider provides a generic Provider Registry + Factory pattern for
// Astra web framework. It standardises how "multi-provider" modules (OAuth
// login providers, payment gateways, SMS channels, storage backends, etc.)
// register and resolve their implementations.
//
// # provider vs di package
//
// This package and the di package address different problems:
//   - di.Container: manages dependency lifecycles and environment-aware injection
//     (wire a *sql.DB into your repository; swap implementations per environment)
//   - provider.Registry: dispatches between named implementations of the same
//     interface by code (e.g. choose "alipay" vs "wechat" payment gateway at
//     runtime based on config)
//
// They are orthogonal and composable: use di to wire the provider registry itself,
// then use provider to select a concrete implementation at runtime.
//
// # Quick start
//
//	// 1. Define your provider interface
//	type SMSSender interface {
//	    Send(ctx context.Context, to, msg string) error
//	}
//
//	// 2. Implement a factory
//	type AliyunSMSFactory struct{}
//	func (f *AliyunSMSFactory) Code() string { return "aliyun" }
//	func (f *AliyunSMSFactory) New(ctx context.Context, cfg any) (any, error) {
//	    p := cfg.(SMSSenderConfig)
//	    return &aliyunSender{apiKey: p.APIKey}, nil
//	}
//
//	// 3. Register
//	reg := provider.NewRegistry[SMSSender]()
//	provider.MustRegister(reg, &AliyunSMSFactory{})
//
//	// 4. Resolve
//	sender, _ := reg.Get("aliyun")
//	sender.Send(ctx, "138xxx", "hello")
package provider

import (
	"fmt"
	"sync"
)

// Factory creates a provider instance from a raw config value.
// T is the provider interface type. Implementations embed this interface
// to enforce compile-time satisfaction.
type Factory[T any] interface {
	// Code returns a unique identifier for this provider, e.g. "wechat", "alipay".
	Code() string
	// New creates a provider instance. cfg is the provider-specific config
	// (parsed from JSON/YAML or passed programmatically). The caller is
	// responsible for type-asserting the result to T.
	New(ctx any, cfg any) (T, error)
}

// Registry is a thread-safe container for ProviderFactory instances keyed by Code.
type Registry[T any] struct {
	mu     sync.RWMutex
	items  map[string]Factory[T]
	order  []string // insertion order for deterministic iteration
}

// NewRegistry creates an empty provider registry.
func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{
		items: make(map[string]Factory[T]),
	}
}

// Register registers a provider factory. Returns an error if a factory with
// the same Code has already been registered.
func (r *Registry[T]) Register(f Factory[T]) error {
	if f == nil {
		return fmt.Errorf("provider: cannot register nil factory")
	}
	code := f.Code()
	if code == "" {
		return fmt.Errorf("provider: factory Code() must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[code]; exists {
		return fmt.Errorf("provider: factory %q already registered", code)
	}
	r.items[code] = f
	r.order = append(r.order, code)
	return nil
}

// MustRegister registers a provider factory and panics on conflict.
func MustRegister[T any](r *Registry[T], f Factory[T]) {
	if err := r.Register(f); err != nil {
		panic(fmt.Sprintf("provider: MustRegister %q: %v", f.Code(), err))
	}
}

// Get resolves a factory by code. Returns false if not found.
func (r *Registry[T]) Get(code string) (Factory[T], bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.items[code]
	return f, ok
}

// List returns all registered provider codes in insertion order.
func (r *Registry[T]) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Create creates a provider instance for the given code using its factory.
// The cfg is passed to the factory's New method.
func (r *Registry[T]) Create(ctx any, code string, cfg any) (T, error) {
	var zero T
	f, ok := r.Get(code)
	if !ok {
		return zero, fmt.Errorf("provider: %q not found (registered: %v)", code, r.List())
	}
	inst, err := f.New(ctx, cfg)
	if err != nil {
		return zero, fmt.Errorf("provider: %q New: %w", code, err)
	}
	return inst, nil
}

// MustCreate calls Create and panics on error.
func (r *Registry[T]) MustCreate(ctx any, code string, cfg any) T {
	inst, err := r.Create(ctx, code, cfg)
	if err != nil {
		panic(fmt.Sprintf("provider: MustCreate %q: %v", code, err))
	}
	return inst
}

// Count returns the number of registered providers.
func (r *Registry[T]) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// ForEach calls fn for each registered factory in insertion order.
// If fn returns an error, iteration stops and the error is returned.
func (r *Registry[T]) ForEach(fn func(code string, f Factory[T]) error) error {
	r.mu.RLock()
	codes := make([]string, len(r.order))
	copy(codes, r.order)

	factories := make(map[string]Factory[T], len(r.items))
	for k, v := range r.items {
		factories[k] = v
	}
	r.mu.RUnlock()

	for _, code := range codes {
		if err := fn(code, factories[code]); err != nil {
			return err
		}
	}
	return nil
}

// Reset clears all registered providers (primarily for testing).
func (r *Registry[T]) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = make(map[string]Factory[T])
	r.order = nil
}
