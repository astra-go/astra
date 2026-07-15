// Package di — provider registry for runtime dispatch
//
// ProviderRegistry dispatches between named implementations of the same
// interface at runtime (the "Strategy pattern at scale").
//
// # When to use ProviderRegistry vs Container
//
//	di.Container        → wire dependencies at startup
//	                       (DB → Repo → Service → Handler)
//	di.ProviderSet      → pick the right dependency graph per environment
//	                       (dev vs prod sets of the same services)
//	di.ProviderRegistry → choose a concrete strategy at runtime by code
//	                       ("alipay" vs "wechat", "aws-s3" vs "gcs")
//
// Container and ProviderRegistry are orthogonal and composable:
//
//	c := di.New()
//
//	// Use Container to wire the registry itself.
//	di.ProvideValue(c, di.NewProviderRegistry[PaymentGateway]())
//
//	// Use ProviderRegistry to select a strategy at runtime.
//	reg := di.MustInvoke[*di.ProviderRegistry[PaymentGateway]](c)
//	gateway, _ := reg.Create(ctx, "wechat", wechatCfg)
package di

import (
	"fmt"
	"sync"
)

// ProviderFactory creates a provider instance from a raw config value.
// T is the provider interface type.
//
// Implementations embed this interface to enforce compile-time satisfaction
// and register with ProviderRegistry:
//
//	type AliyunSMS struct{}
//	func (f *AliyunSMS) Code() string               { return "aliyun" }
//	func (f *AliyunSMS) New(ctx any, cfg any) (SMSSender, error) {
//	    c := cfg.(*AliyunConfig)
//	    return &aliyunSender{key: c.APIKey}, nil
//	}
//
//	// Compile-time check
//	var _ di.ProviderFactory[SMSSender] = (*AliyunSMS)(nil)
type ProviderFactory[T any] interface {
	// Code returns a unique identifier for this provider (e.g. "wechat",
	// "alipay", "aws-s3").  Code must not be empty.
	Code() string

	// New creates a provider instance. cfg is the provider-specific
	// configuration (parsed from JSON/YAML or passed programmatically).
	// The caller is responsible for type-asserting the result to T.
	New(ctx any, cfg any) (T, error)
}

// ProviderRegistry is a thread-safe container of ProviderFactory instances
// keyed by Code.  Use it to dispatch between implementations of the same
// interface at runtime based on configuration.
//
//	reg := di.NewProviderRegistry[SMSSender]()
//	reg.Register(&AliyunSMS{})
//	reg.Register(&TwilioSMS{})
//
//	// Later, in a request handler:
//	sender, _ := reg.Create(ctx, "aliyun", aliyunCfg)
//	sender.Send(ctx, to, msg)
type ProviderRegistry[T any] struct {
	mu    sync.RWMutex
	items map[string]ProviderFactory[T]
	order []string // insertion order for deterministic iteration
}

// NewProviderRegistry creates an empty provider registry.
func NewProviderRegistry[T any]() *ProviderRegistry[T] {
	return &ProviderRegistry[T]{
		items: make(map[string]ProviderFactory[T]),
	}
}

// Register registers a provider factory. Returns an error if a factory with
// the same Code has already been registered.
func (r *ProviderRegistry[T]) Register(f ProviderFactory[T]) error {
	if f == nil {
		return fmt.Errorf("di: cannot register nil ProviderFactory")
	}
	code := f.Code()
	if code == "" {
		return fmt.Errorf("di: ProviderFactory.Code() must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[code]; exists {
		return fmt.Errorf("di: ProviderFactory %q already registered", code)
	}
	r.items[code] = f
	r.order = append(r.order, code)
	return nil
}

// MustRegister registers a provider factory and panics on conflict.
func (r *ProviderRegistry[T]) MustRegister(f ProviderFactory[T]) {
	if err := r.Register(f); err != nil {
		panic(fmt.Sprintf("di: MustRegister %q: %v", f.Code(), err))
	}
}

// Get resolves a factory by code. Returns false if not found.
func (r *ProviderRegistry[T]) Get(code string) (ProviderFactory[T], bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.items[code]
	return f, ok
}

// List returns all registered provider codes in insertion order.
func (r *ProviderRegistry[T]) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Create creates a provider instance for the given code using its factory.
func (r *ProviderRegistry[T]) Create(ctx any, code string, cfg any) (T, error) {
	var zero T
	f, ok := r.Get(code)
	if !ok {
		return zero, fmt.Errorf("di: provider %q not found (registered: %v)", code, r.List())
	}
	inst, err := f.New(ctx, cfg)
	if err != nil {
		return zero, fmt.Errorf("di: provider %q New: %w", code, err)
	}
	return inst, nil
}

// MustCreate calls Create and panics on error.
func (r *ProviderRegistry[T]) MustCreate(ctx any, code string, cfg any) T {
	inst, err := r.Create(ctx, code, cfg)
	if err != nil {
		panic(fmt.Sprintf("di: MustCreate %q: %v", code, err))
	}
	return inst
}

// Count returns the number of registered providers.
//
// Alias: Len (preferred for consistency with Container.Len).
func (r *ProviderRegistry[T]) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// Len is an alias for Count. Provided for consistency with Container.Len.
func (r *ProviderRegistry[T]) Len() int { return r.Count() }

// ForEach calls fn for each registered factory in insertion order.
// If fn returns an error, iteration stops and the error is returned.
func (r *ProviderRegistry[T]) ForEach(fn func(code string, f ProviderFactory[T]) error) error {
	r.mu.RLock()
	codes := make([]string, len(r.order))
	copy(codes, r.order)

	factories := make(map[string]ProviderFactory[T], len(r.items))
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
func (r *ProviderRegistry[T]) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = make(map[string]ProviderFactory[T])
	r.order = nil
}
