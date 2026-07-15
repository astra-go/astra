// Deprecated: Package provider is superseded by di.ProviderRegistry and
// di.ProviderFactory. The two packages address orthogonal concerns:
//
//	di.Container        → wire dependencies at startup
//	di.ProviderRegistry → dispatch between named implementations at runtime
//
// Migrate by changing imports:
//
//	// Old
//	import "github.com/astra-go/astra/provider"
//	reg := provider.NewRegistry[MyInterface]()
//	provider.MustRegister(reg, factory)
//
//	// New
//	import "github.com/astra-go/astra/di"
//	reg := di.NewProviderRegistry[MyInterface]()
//	reg.MustRegister(factory)
//
// Removal timeline: This package will be removed in v1.1.1.
// Users are encouraged to migrate before then.
package provider

import "github.com/astra-go/astra/di"

// Factory is an alias for di.ProviderFactory.
//
// Deprecated: Use di.ProviderFactory instead.
type Factory[T any] = di.ProviderFactory[T]

// Registry is an alias for di.ProviderRegistry.
//
// Deprecated: Use di.ProviderRegistry instead.
type Registry[T any] = di.ProviderRegistry[T]

// NewRegistry creates a new provider registry.
//
// Deprecated: Use di.NewProviderRegistry instead.
func NewRegistry[T any]() *Registry[T] {
	return di.NewProviderRegistry[T]()
}

// MustRegister registers a factory and panics on conflict.
//
// Deprecated: Use (*di.ProviderRegistry).MustRegister instead.
func MustRegister[T any](r *Registry[T], f Factory[T]) {
	r.MustRegister(f)
}
