package di

import (
	"errors"
	"testing"
)

// TestProviderFunc tests the generic ProviderFunc type.
func TestProviderFunc(t *testing.T) {
	c := New()

	// Register using ProviderFunc
	set := NewSet("test",
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return &simpleService{name: "test"}, nil
		}),
	)

	if err := set.Apply(c); err != nil {
		t.Fatalf("failed to apply set: %v", err)
	}

	// Resolve and verify
	svc, err := Invoke[*simpleService](c)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}
	if svc.name != "test" {
		t.Fatalf("expected name 'test', got %q", svc.name)
	}
}

// TestFactoryFunc tests the generic FactoryFunc type with Container.
func TestFactoryFunc(t *testing.T) {
	c := New()

	// First register the dependency
	ProvideValue(c, &dependencyService{name: "dep"})

	// Register using FactoryFunc
	set := NewSet("test",
		FactoryFunc[*complexService](func(c *Container) (*complexService, error) {
			dep, _ := Invoke[*dependencyService](c)
			return &complexService{dep: dep, name: "complex"}, nil
		}),
	)

	if err := set.Apply(c); err != nil {
		t.Fatalf("failed to apply set: %v", err)
	}

	// Resolve and verify
	svc, err := Invoke[*complexService](c)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}
	if svc.name != "complex" {
		t.Fatalf("expected name 'complex', got %q", svc.name)
	}
	if svc.dep == nil || svc.dep.name != "dep" {
		t.Fatal("dependency not injected correctly")
	}
}

// TestProviderFuncWithError tests ProviderFunc that returns an error.
func TestProviderFuncWithError(t *testing.T) {
	c := New()

	wantErr := errors.New("factory error")
	set := NewSet("test",
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return nil, wantErr
		}),
	)

	if err := set.Apply(c); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestMixedProviders tests mixing ProviderFunc, FactoryFunc, and legacy forms.
func TestMixedProviders(t *testing.T) {
	c := New()

	// Legacy form
	ProvideValue(c, &dependencyService{name: "legacy-dep"})

	set := NewSet("test",
		// ProviderFunc (simple, no dependencies)
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return &simpleService{name: "simple"}, nil
		}),
		// FactoryFunc (with dependencies)
		FactoryFunc[*complexService](func(c *Container) (*complexService, error) {
			dep, _ := Invoke[*dependencyService](c)
			return &complexService{dep: dep, name: "from-factory"}, nil
		}),
		// Legacy form
		func(c *Container) error {
			return Provide(c, func(_ *Container) (*legacyService, error) {
				return &legacyService{val: 42}, nil
			})
		},
	)

	if err := set.Apply(c); err != nil {
		t.Fatalf("failed to apply set: %v", err)
	}

	// Verify all registrations
	simple, err := Invoke[*simpleService](c)
	if err != nil || simple.name != "simple" {
		t.Fatalf("simpleService failed: err=%v, name=%q", err, simple.name)
	}

	complex, err := Invoke[*complexService](c)
	if err != nil || complex.name != "from-factory" {
		t.Fatalf("complexService failed: err=%v, name=%q", err, complex.name)
	}

	legacy, err := Invoke[*legacyService](c)
	if err != nil || legacy.val != 42 {
		t.Fatalf("legacyService failed: err=%v, val=%d", err, legacy.val)
	}
}

// TestNewSetInference tests that NewSet accepts variadic any.
func TestNewSetInference(t *testing.T) {
	// ProviderFunc should be inferrable
	_ = NewSet("dev",
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return &simpleService{name: "inferred"}, nil
		}),
	)

	// FactoryFunc should be inferrable
	_ = NewSet("prod",
		FactoryFunc[*simpleService](func(c *Container) (*simpleService, error) {
			return &simpleService{name: "factory-inferred"}, nil
		}),
	)
}

// TestProviderSetWithGenericFunctions tests RegisterSets with generic functions.
func TestProviderSetWithGenericFunctions(t *testing.T) {
	c := New()

	devSet := NewSet("dev",
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return &simpleService{name: "dev-service"}, nil
		}),
	)

	prodSet := NewSet("prod",
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return &simpleService{name: "prod-service"}, nil
		}),
	)

	// Register dev environment
	MustRegisterSets(c, "dev", devSet)

	svc, err := Invoke[*simpleService](c)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}
	if svc.name != "dev-service" {
		t.Fatalf("expected 'dev-service', got %q", svc.name)
	}

	// Create new container with prod
	c2 := New()
	MustRegisterSets(c2, "prod", devSet, prodSet)

	svc2, err := Invoke[*simpleService](c2)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}
	if svc2.name != "prod-service" {
		t.Fatalf("expected 'prod-service', got %q", svc2.name)
	}
}

// TestProviderSetGenericTransitive tests transitive dependencies with generic functions.
func TestProviderSetGenericTransitive(t *testing.T) {
	c := New()

	// Layer 1: base dependency
	ProvideValue(c, &dependencyService{name: "base"})

	// Layer 2: uses base
	_ = NewSet("test",
		FactoryFunc[*serviceA](func(c *Container) (*serviceA, error) {
			dep, _ := Invoke[*dependencyService](c)
			return &serviceA{dep: dep}, nil
		}),
	).Apply(c)

	// Layer 3: uses serviceA
	_ = NewSet("test",
		FactoryFunc[*serviceB](func(c *Container) (*serviceB, error) {
			a, _ := Invoke[*serviceA](c)
			return &serviceB{a: a}, nil
		}),
	).Apply(c)

	// Resolve and verify chain
	b, err := Invoke[*serviceB](c)
	if err != nil {
		t.Fatalf("failed to invoke serviceB: %v", err)
	}
	if b.a == nil || b.a.dep == nil || b.a.dep.name != "base" {
		t.Fatal("transitive dependency chain broken")
	}
}

// Helper types for tests
type simpleService struct {
	name string
}

type dependencyService struct {
	name string
}

type complexService struct {
	dep  *dependencyService
	name string
}

type legacyService struct {
	val int
}

type serviceA struct {
	dep *dependencyService
}

type serviceB struct {
	a *serviceA
}

// TestDefaultSetWithGenericFunctions tests NewDefaultSet with generic functions.
func TestDefaultSetWithGenericFunctions(t *testing.T) {
	c := New()

	defaultSet := NewDefaultSet(
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			return &simpleService{name: "default"}, nil
		}),
	)

	if err := defaultSet.Apply(c); err != nil {
		t.Fatalf("failed to apply default set: %v", err)
	}

	svc, err := Invoke[*simpleService](c)
	if err != nil || svc.name != "default" {
		t.Fatalf("default set not applied: err=%v, name=%q", err, svc.name)
	}
}

// TestProviderFunc_CalledOnce verifies that the provider function is invoked
// exactly once per registration, not twice (regression test for the fix that
// removed the redundant v.Call in toRegistrar).
func TestProviderFunc_CalledOnce(t *testing.T) {
	c := New()
	calls := 0

	set := NewSet("test",
		ProviderFunc[*simpleService](func() (*simpleService, error) {
			calls++
			return &simpleService{name: "once"}, nil
		}),
	)

	if err := set.Apply(c); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Resolve twice — the factory itself runs only during Apply.
	Invoke[*simpleService](c)
	Invoke[*simpleService](c)

	if calls != 1 {
		t.Fatalf("expected provider called 1 time (during Apply), got %d", calls)
	}
}

// TestFactoryFunc_CalledOnce verifies that the factory function is invoked
// exactly once per registration, not twice.
func TestFactoryFunc_CalledOnce(t *testing.T) {
	c := New()
	ProvideValue(c, &dependencyService{name: "dep"})
	calls := 0

	set := NewSet("test",
		FactoryFunc[*complexService](func(c *Container) (*complexService, error) {
			calls++
			dep, _ := Invoke[*dependencyService](c)
			return &complexService{dep: dep, name: "once"}, nil
		}),
	)

	if err := set.Apply(c); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Resolve twice — the factory itself runs only during Apply.
	Invoke[*complexService](c)
	Invoke[*complexService](c)

	if calls != 1 {
		t.Fatalf("expected factory called 1 time (during Apply), got %d", calls)
	}
}
