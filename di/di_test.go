package di_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/astra-go/astra/di"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

type DB struct{ Name string }
type Cache interface{ Get(string) string }

type memCache struct{ prefix string }

func (m *memCache) Get(k string) string { return m.prefix + k }

// ─── Provide / Invoke ─────────────────────────────────────────────────────────

func TestProvideAndInvoke(t *testing.T) {
	c := di.New()

	if err := di.Provide[*DB](c, func(_ *di.Container) (*DB, error) {
		return &DB{Name: "main"}, nil
	}); err != nil {
		t.Fatal(err)
	}

	db, err := di.Invoke[*DB](c)
	if err != nil {
		t.Fatal(err)
	}
	if db.Name != "main" {
		t.Fatalf("expected 'main', got %q", db.Name)
	}
}

func TestSingleton(t *testing.T) {
	c := di.New()
	calls := 0
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) {
		calls++
		return &DB{Name: "singleton"}, nil
	})

	a, _ := di.Invoke[*DB](c)
	b, _ := di.Invoke[*DB](c)
	if a != b {
		t.Fatal("expected same pointer")
	}
	if calls != 1 {
		t.Fatalf("factory called %d times, want 1", calls)
	}
}

func TestErrNotFound(t *testing.T) {
	c := di.New()
	_, err := di.Invoke[*DB](c)
	if !errors.Is(err, di.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestErrDuplicate(t *testing.T) {
	c := di.New()
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) { return &DB{}, nil })
	err := di.Provide[*DB](c, func(_ *di.Container) (*DB, error) { return &DB{}, nil })
	if !errors.Is(err, di.ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestFactoryError(t *testing.T) {
	c := di.New()
	want := errors.New("connect failed")
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) { return nil, want })

	_, err := di.Invoke[*DB](c)
	if !errors.Is(err, want) {
		t.Fatalf("expected factory error to be wrapped, got %v", err)
	}
}

// ─── ProvideValue ─────────────────────────────────────────────────────────────

func TestProvideValue(t *testing.T) {
	c := di.New()
	_ = di.ProvideValue[*DB](c, &DB{Name: "prebuilt"})
	db := di.MustInvoke[*DB](c)
	if db.Name != "prebuilt" {
		t.Fatalf("unexpected %q", db.Name)
	}
}

// ─── Named instances ──────────────────────────────────────────────────────────

func TestNamed(t *testing.T) {
	c := di.New()
	_ = di.ProvideNamed[Cache](c, "mem", func(_ *di.Container) (Cache, error) {
		return &memCache{prefix: "mem:"}, nil
	})
	_ = di.ProvideNamed[Cache](c, "noop", func(_ *di.Container) (Cache, error) {
		return &memCache{prefix: "noop:"}, nil
	})

	mem := di.MustInvokeNamed[Cache](c, "mem")
	noop := di.MustInvokeNamed[Cache](c, "noop")

	if got := mem.Get("x"); got != "mem:x" {
		t.Errorf("mem: got %q", got)
	}
	if got := noop.Get("x"); got != "noop:x" {
		t.Errorf("noop: got %q", got)
	}
}

func TestNamedNotFound(t *testing.T) {
	c := di.New()
	_, err := di.InvokeNamed[Cache](c, "missing")
	if !errors.Is(err, di.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// ─── Transitive dependencies ──────────────────────────────────────────────────

func TestTransitiveDependency(t *testing.T) {
	type Service struct{ DB *DB }

	c := di.New()
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) {
		return &DB{Name: "shared"}, nil
	})
	_ = di.Provide[*Service](c, func(c *di.Container) (*Service, error) {
		db, err := di.Invoke[*DB](c)
		if err != nil {
			return nil, err
		}
		return &Service{DB: db}, nil
	})

	svc, err := di.Invoke[*Service](c)
	if err != nil {
		t.Fatal(err)
	}
	if svc.DB.Name != "shared" {
		t.Fatalf("unexpected DB name %q", svc.DB.Name)
	}
}

// ─── Has / HasNamed ───────────────────────────────────────────────────────────

func TestHas(t *testing.T) {
	c := di.New()
	if di.Has[*DB](c) {
		t.Fatal("should not have *DB")
	}
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) { return &DB{}, nil })
	if !di.Has[*DB](c) {
		t.Fatal("should have *DB")
	}
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

func TestLifecycle(t *testing.T) {
	c := di.New()
	var log []string

	c.OnStart(func(_ context.Context) error { log = append(log, "start:1"); return nil })
	c.OnStart(func(_ context.Context) error { log = append(log, "start:2"); return nil })
	c.OnStop(func(_ context.Context) error { log = append(log, "stop:1"); return nil })
	c.OnStop(func(_ context.Context) error { log = append(log, "stop:2"); return nil })

	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// stop hooks run LIFO
	if err := c.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"start:1", "start:2", "stop:2", "stop:1"}
	for i, w := range want {
		if log[i] != w {
			t.Fatalf("step %d: want %q, got %q", i, w, log[i])
		}
	}
}

func TestStartError(t *testing.T) {
	c := di.New()
	boom := errors.New("boom")
	called := false

	c.OnStart(func(_ context.Context) error { return boom })
	c.OnStart(func(_ context.Context) error { called = true; return nil })

	err := c.Start(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if called {
		t.Fatal("second hook should not have run after first failed")
	}
}

func TestStopRunsAllHooks(t *testing.T) {
	c := di.New()
	boom := errors.New("first-stop-error")
	secondRan := false

	c.OnStop(func(_ context.Context) error { return boom })
	c.OnStop(func(_ context.Context) error { secondRan = true; return nil })

	err := c.Stop(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	// Both hooks must run even when one fails (LIFO: second runs first here)
	if !secondRan {
		t.Fatal("second stop hook should have run")
	}
}

// ─── Len ──────────────────────────────────────────────────────────────────────

func TestLen(t *testing.T) {
	c := di.New()
	if c.Len() != 0 {
		t.Fatalf("expected 0, got %d", c.Len())
	}
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) { return &DB{}, nil })
	_ = di.ProvideNamed[Cache](c, "mem", func(_ *di.Container) (Cache, error) { return &memCache{}, nil })
	if c.Len() != 2 {
		t.Fatalf("expected 2, got %d", c.Len())
	}
}

// ─── Circular dependency detection ────────────────────��──────────────────────

// assertCyclePanic calls fn and asserts that it panics with ErrCyclicDependency.
// The test fails (not panics) if no panic occurs or the panic value is wrong.
func assertCyclePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for circular dependency, got none (possible deadlock)")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T: %v", r, r)
		}
		if !errors.Is(err, di.ErrCyclicDependency) {
			t.Fatalf("expected ErrCyclicDependency, got: %v", err)
		}
	}()
	fn()
}

// TestCyclicDependency_TwoWay: A → B → A.
func TestCyclicDependency_TwoWay(t *testing.T) {
	type SvcA struct{}
	type SvcB struct{}

	c := di.New()
	_ = di.Provide[*SvcA](c, func(c *di.Container) (*SvcA, error) {
		_, _ = di.Invoke[*SvcB](c)
		return &SvcA{}, nil
	})
	_ = di.Provide[*SvcB](c, func(c *di.Container) (*SvcB, error) {
		_, _ = di.Invoke[*SvcA](c)
		return &SvcB{}, nil
	})

	assertCyclePanic(t, func() { di.MustInvoke[*SvcA](c) })
}

// TestCyclicDependency_ThreeWay: A → B → C → A.
func TestCyclicDependency_ThreeWay(t *testing.T) {
	type SvcA struct{}
	type SvcB struct{}
	type SvcC struct{}

	c := di.New()
	_ = di.Provide[*SvcA](c, func(c *di.Container) (*SvcA, error) {
		_, _ = di.Invoke[*SvcB](c)
		return &SvcA{}, nil
	})
	_ = di.Provide[*SvcB](c, func(c *di.Container) (*SvcB, error) {
		_, _ = di.Invoke[*SvcC](c)
		return &SvcB{}, nil
	})
	_ = di.Provide[*SvcC](c, func(c *di.Container) (*SvcC, error) {
		_, _ = di.Invoke[*SvcA](c)
		return &SvcC{}, nil
	})

	assertCyclePanic(t, func() { di.MustInvoke[*SvcA](c) })
}

// TestCyclicDependency_PanicMessage verifies that the panic value wraps
// ErrCyclicDependency and that the message includes a readable cycle path.
func TestCyclicDependency_PanicMessage(t *testing.T) {
	type SvcA struct{}
	type SvcB struct{}

	c := di.New()
	_ = di.Provide[*SvcA](c, func(c *di.Container) (*SvcA, error) {
		_, _ = di.Invoke[*SvcB](c)
		return &SvcA{}, nil
	})
	_ = di.Provide[*SvcB](c, func(c *di.Container) (*SvcB, error) {
		_, _ = di.Invoke[*SvcA](c)
		return &SvcB{}, nil
	})

	var panicVal any
	func() {
		defer func() { panicVal = recover() }()
		di.MustInvoke[*SvcA](c)
	}()

	err, ok := panicVal.(error)
	if !ok {
		t.Fatalf("expected error panic, got %T: %v", panicVal, panicVal)
	}
	if !errors.Is(err, di.ErrCyclicDependency) {
		t.Fatalf("expected ErrCyclicDependency, got: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "→") {
		t.Errorf("cycle path should contain '→', got: %q", msg)
	}
	t.Logf("cycle message: %s", msg)
}

// TestNoCyclicDependency_Concurrent verifies that two goroutines resolving
// the same type simultaneously does NOT trigger a false cycle alarm.
func TestNoCyclicDependency_Concurrent(t *testing.T) {
	c := di.New()
	_ = di.Provide[*DB](c, func(_ *di.Container) (*DB, error) {
		return &DB{Name: "shared"}, nil
	})

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					errs <- errors.New("unexpected panic")
				} else {
					errs <- nil
				}
			}()
			_, _ = di.Invoke[*DB](c)
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

// ─── Maximum depth detection ──────────────────────────────────────────────────

// assertDepthPanic calls fn and asserts that it panics with ErrMaxDepthExceeded.
// The test fails (not panics) if no panic occurs or the panic value is wrong.
func assertDepthPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for depth exceeded, got none")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error panic, got %T: %v", r, r)
		}
		if !errors.Is(err, di.ErrMaxDepthExceeded) {
			t.Fatalf("expected ErrMaxDepthExceeded, got: %v", err)
		}
	}()
	fn()
}

// TestMaxDepth_Boundary verifies that the default maxDepth=32 allows 32 layers
// but panics at 33.
func TestMaxDepth_Boundary(t *testing.T) {
	c := di.New()

	// Build a chain of 33 services: S0 → S1 → S2 → ... → S32
	type S0 struct{}
	type S1 struct{}
	type S2 struct{}
	type S3 struct{}
	type S4 struct{}
	type S5 struct{}
	type S6 struct{}
	type S7 struct{}
	type S8 struct{}
	type S9 struct{}
	type S10 struct{}
	type S11 struct{}
	type S12 struct{}
	type S13 struct{}
	type S14 struct{}
	type S15 struct{}
	type S16 struct{}
	type S17 struct{}
	type S18 struct{}
	type S19 struct{}
	type S20 struct{}
	type S21 struct{}
	type S22 struct{}
	type S23 struct{}
	type S24 struct{}
	type S25 struct{}
	type S26 struct{}
	type S27 struct{}
	type S28 struct{}
	type S29 struct{}
	type S30 struct{}
	type S31 struct{}
	type S32 struct{}

	_ = di.Provide[*S32](c, func(_ *di.Container) (*S32, error) { return &S32{}, nil })
	_ = di.Provide[*S31](c, func(c *di.Container) (*S31, error) { _, _ = di.Invoke[*S32](c); return &S31{}, nil })
	_ = di.Provide[*S30](c, func(c *di.Container) (*S30, error) { _, _ = di.Invoke[*S31](c); return &S30{}, nil })
	_ = di.Provide[*S29](c, func(c *di.Container) (*S29, error) { _, _ = di.Invoke[*S30](c); return &S29{}, nil })
	_ = di.Provide[*S28](c, func(c *di.Container) (*S28, error) { _, _ = di.Invoke[*S29](c); return &S28{}, nil })
	_ = di.Provide[*S27](c, func(c *di.Container) (*S27, error) { _, _ = di.Invoke[*S28](c); return &S27{}, nil })
	_ = di.Provide[*S26](c, func(c *di.Container) (*S26, error) { _, _ = di.Invoke[*S27](c); return &S26{}, nil })
	_ = di.Provide[*S25](c, func(c *di.Container) (*S25, error) { _, _ = di.Invoke[*S26](c); return &S25{}, nil })
	_ = di.Provide[*S24](c, func(c *di.Container) (*S24, error) { _, _ = di.Invoke[*S25](c); return &S24{}, nil })
	_ = di.Provide[*S23](c, func(c *di.Container) (*S23, error) { _, _ = di.Invoke[*S24](c); return &S23{}, nil })
	_ = di.Provide[*S22](c, func(c *di.Container) (*S22, error) { _, _ = di.Invoke[*S23](c); return &S22{}, nil })
	_ = di.Provide[*S21](c, func(c *di.Container) (*S21, error) { _, _ = di.Invoke[*S22](c); return &S21{}, nil })
	_ = di.Provide[*S20](c, func(c *di.Container) (*S20, error) { _, _ = di.Invoke[*S21](c); return &S20{}, nil })
	_ = di.Provide[*S19](c, func(c *di.Container) (*S19, error) { _, _ = di.Invoke[*S20](c); return &S19{}, nil })
	_ = di.Provide[*S18](c, func(c *di.Container) (*S18, error) { _, _ = di.Invoke[*S19](c); return &S18{}, nil })
	_ = di.Provide[*S17](c, func(c *di.Container) (*S17, error) { _, _ = di.Invoke[*S18](c); return &S17{}, nil })
	_ = di.Provide[*S16](c, func(c *di.Container) (*S16, error) { _, _ = di.Invoke[*S17](c); return &S16{}, nil })
	_ = di.Provide[*S15](c, func(c *di.Container) (*S15, error) { _, _ = di.Invoke[*S16](c); return &S15{}, nil })
	_ = di.Provide[*S14](c, func(c *di.Container) (*S14, error) { _, _ = di.Invoke[*S15](c); return &S14{}, nil })
	_ = di.Provide[*S13](c, func(c *di.Container) (*S13, error) { _, _ = di.Invoke[*S14](c); return &S13{}, nil })
	_ = di.Provide[*S12](c, func(c *di.Container) (*S12, error) { _, _ = di.Invoke[*S13](c); return &S12{}, nil })
	_ = di.Provide[*S11](c, func(c *di.Container) (*S11, error) { _, _ = di.Invoke[*S12](c); return &S11{}, nil })
	_ = di.Provide[*S10](c, func(c *di.Container) (*S10, error) { _, _ = di.Invoke[*S11](c); return &S10{}, nil })
	_ = di.Provide[*S9](c, func(c *di.Container) (*S9, error) { _, _ = di.Invoke[*S10](c); return &S9{}, nil })
	_ = di.Provide[*S8](c, func(c *di.Container) (*S8, error) { _, _ = di.Invoke[*S9](c); return &S8{}, nil })
	_ = di.Provide[*S7](c, func(c *di.Container) (*S7, error) { _, _ = di.Invoke[*S8](c); return &S7{}, nil })
	_ = di.Provide[*S6](c, func(c *di.Container) (*S6, error) { _, _ = di.Invoke[*S7](c); return &S6{}, nil })
	_ = di.Provide[*S5](c, func(c *di.Container) (*S5, error) { _, _ = di.Invoke[*S6](c); return &S5{}, nil })
	_ = di.Provide[*S4](c, func(c *di.Container) (*S4, error) { _, _ = di.Invoke[*S5](c); return &S4{}, nil })
	_ = di.Provide[*S3](c, func(c *di.Container) (*S3, error) { _, _ = di.Invoke[*S4](c); return &S3{}, nil })
	_ = di.Provide[*S2](c, func(c *di.Container) (*S2, error) { _, _ = di.Invoke[*S3](c); return &S2{}, nil })
	_ = di.Provide[*S1](c, func(c *di.Container) (*S1, error) { _, _ = di.Invoke[*S2](c); return &S1{}, nil })
	_ = di.Provide[*S0](c, func(c *di.Container) (*S0, error) { _, _ = di.Invoke[*S1](c); return &S0{}, nil })

	// Depth 31: S1 → S2 → ... → S32 (31 dependencies, 32 total including S1)
	// Should succeed because maxDepth=32 allows up to 32 items on the stack.
	c32 := di.New()
	_ = di.Provide[*S32](c32, func(_ *di.Container) (*S32, error) { return &S32{}, nil })
	_ = di.Provide[*S31](c32, func(c *di.Container) (*S31, error) { _, _ = di.Invoke[*S32](c32); return &S31{}, nil })
	_ = di.Provide[*S30](c32, func(c *di.Container) (*S30, error) { _, _ = di.Invoke[*S31](c32); return &S30{}, nil })
	_ = di.Provide[*S29](c32, func(c *di.Container) (*S29, error) { _, _ = di.Invoke[*S30](c32); return &S29{}, nil })
	_ = di.Provide[*S28](c32, func(c *di.Container) (*S28, error) { _, _ = di.Invoke[*S29](c32); return &S28{}, nil })
	_ = di.Provide[*S27](c32, func(c *di.Container) (*S27, error) { _, _ = di.Invoke[*S28](c32); return &S27{}, nil })
	_ = di.Provide[*S26](c32, func(c *di.Container) (*S26, error) { _, _ = di.Invoke[*S27](c32); return &S26{}, nil })
	_ = di.Provide[*S25](c32, func(c *di.Container) (*S25, error) { _, _ = di.Invoke[*S26](c32); return &S25{}, nil })
	_ = di.Provide[*S24](c32, func(c *di.Container) (*S24, error) { _, _ = di.Invoke[*S25](c32); return &S24{}, nil })
	_ = di.Provide[*S23](c32, func(c *di.Container) (*S23, error) { _, _ = di.Invoke[*S24](c32); return &S23{}, nil })
	_ = di.Provide[*S22](c32, func(c *di.Container) (*S22, error) { _, _ = di.Invoke[*S23](c32); return &S22{}, nil })
	_ = di.Provide[*S21](c32, func(c *di.Container) (*S21, error) { _, _ = di.Invoke[*S22](c32); return &S21{}, nil })
	_ = di.Provide[*S20](c32, func(c *di.Container) (*S20, error) { _, _ = di.Invoke[*S21](c32); return &S20{}, nil })
	_ = di.Provide[*S19](c32, func(c *di.Container) (*S19, error) { _, _ = di.Invoke[*S20](c32); return &S19{}, nil })
	_ = di.Provide[*S18](c32, func(c *di.Container) (*S18, error) { _, _ = di.Invoke[*S19](c32); return &S18{}, nil })
	_ = di.Provide[*S17](c32, func(c *di.Container) (*S17, error) { _, _ = di.Invoke[*S18](c32); return &S17{}, nil })
	_ = di.Provide[*S16](c32, func(c *di.Container) (*S16, error) { _, _ = di.Invoke[*S17](c32); return &S16{}, nil })
	_ = di.Provide[*S15](c32, func(c *di.Container) (*S15, error) { _, _ = di.Invoke[*S16](c32); return &S15{}, nil })
	_ = di.Provide[*S14](c32, func(c *di.Container) (*S14, error) { _, _ = di.Invoke[*S15](c32); return &S14{}, nil })
	_ = di.Provide[*S13](c32, func(c *di.Container) (*S13, error) { _, _ = di.Invoke[*S14](c32); return &S13{}, nil })
	_ = di.Provide[*S12](c32, func(c *di.Container) (*S12, error) { _, _ = di.Invoke[*S13](c32); return &S12{}, nil })
	_ = di.Provide[*S11](c32, func(c *di.Container) (*S11, error) { _, _ = di.Invoke[*S12](c32); return &S11{}, nil })
	_ = di.Provide[*S10](c32, func(c *di.Container) (*S10, error) { _, _ = di.Invoke[*S11](c32); return &S10{}, nil })
	_ = di.Provide[*S9](c32, func(c *di.Container) (*S9, error) { _, _ = di.Invoke[*S10](c32); return &S9{}, nil })
	_ = di.Provide[*S8](c32, func(c *di.Container) (*S8, error) { _, _ = di.Invoke[*S9](c32); return &S8{}, nil })
	_ = di.Provide[*S7](c32, func(c *di.Container) (*S7, error) { _, _ = di.Invoke[*S8](c32); return &S7{}, nil })
	_ = di.Provide[*S6](c32, func(c *di.Container) (*S6, error) { _, _ = di.Invoke[*S7](c32); return &S6{}, nil })
	_ = di.Provide[*S5](c32, func(c *di.Container) (*S5, error) { _, _ = di.Invoke[*S6](c32); return &S5{}, nil })
	_ = di.Provide[*S4](c32, func(c *di.Container) (*S4, error) { _, _ = di.Invoke[*S5](c32); return &S4{}, nil })
	_ = di.Provide[*S3](c32, func(c *di.Container) (*S3, error) { _, _ = di.Invoke[*S4](c32); return &S3{}, nil })
	_ = di.Provide[*S2](c32, func(c *di.Container) (*S2, error) { _, _ = di.Invoke[*S3](c32); return &S2{}, nil })
	_ = di.Provide[*S1](c32, func(c *di.Container) (*S1, error) { _, _ = di.Invoke[*S2](c32); return &S1{}, nil })
	_, err := di.Invoke[*S1](c32)
	if err != nil {
		t.Fatalf("depth 31 should succeed, got: %v", err)
	}

	// Depth 32: S0 → S1 → ... → S32 (32 dependencies, 33 total including S0)
	// Should panic because stack length would be 33, exceeding maxDepth=32.
	assertDepthPanic(t, func() { di.MustInvoke[*S0](c) })
}

// TestMaxDepth_ErrorMessage verifies that the depth-exceeded error message
// contains a readable dependency chain.
func TestMaxDepth_ErrorMessage(t *testing.T) {
	type SvcA struct{}
	type SvcB struct{}
	type SvcC struct{}

	c := di.New().WithMaxDepth(2)
	_ = di.Provide[*SvcC](c, func(_ *di.Container) (*SvcC, error) { return &SvcC{}, nil })
	_ = di.Provide[*SvcB](c, func(c *di.Container) (*SvcB, error) { _, _ = di.Invoke[*SvcC](c); return &SvcB{}, nil })
	_ = di.Provide[*SvcA](c, func(c *di.Container) (*SvcA, error) { _, _ = di.Invoke[*SvcB](c); return &SvcA{}, nil })

	var panicVal any
	func() {
		defer func() { panicVal = recover() }()
		di.MustInvoke[*SvcA](c)
	}()

	err, ok := panicVal.(error)
	if !ok {
		t.Fatalf("expected error panic, got %T: %v", panicVal, panicVal)
	}
	if !errors.Is(err, di.ErrMaxDepthExceeded) {
		t.Fatalf("expected ErrMaxDepthExceeded, got: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "→") {
		t.Errorf("depth path should contain '→', got: %q", msg)
	}
	if !strings.Contains(msg, "limit: 2") {
		t.Errorf("error should mention limit, got: %q", msg)
	}
	t.Logf("depth error message: %s", msg)
}

// TestMaxDepth_Configurable verifies that WithMaxDepth allows customizing
// the depth limit.
func TestMaxDepth_Configurable(t *testing.T) {
	type SvcA struct{}
	type SvcB struct{}

	// maxDepth=0 means unlimited
	c0 := di.New().WithMaxDepth(0)
	_ = di.Provide[*SvcB](c0, func(_ *di.Container) (*SvcB, error) { return &SvcB{}, nil })
	_ = di.Provide[*SvcA](c0, func(c *di.Container) (*SvcA, error) { _, _ = di.Invoke[*SvcB](c); return &SvcA{}, nil })
	_, err := di.Invoke[*SvcA](c0)
	if err != nil {
		t.Fatalf("maxDepth=0 should allow unlimited depth, got: %v", err)
	}

	// maxDepth=1 should panic at depth 1
	c1 := di.New().WithMaxDepth(1)
	_ = di.Provide[*SvcB](c1, func(_ *di.Container) (*SvcB, error) { return &SvcB{}, nil })
	_ = di.Provide[*SvcA](c1, func(c *di.Container) (*SvcA, error) { _, _ = di.Invoke[*SvcB](c); return &SvcA{}, nil })
	assertDepthPanic(t, func() { di.MustInvoke[*SvcA](c1) })

	// maxDepth=50 should allow deep chains
	c50 := di.New().WithMaxDepth(50)
	_ = di.Provide[*SvcB](c50, func(_ *di.Container) (*SvcB, error) { return &SvcB{}, nil })
	_ = di.Provide[*SvcA](c50, func(c *di.Container) (*SvcA, error) { _, _ = di.Invoke[*SvcB](c); return &SvcA{}, nil })
	_, err = di.Invoke[*SvcA](c50)
	if err != nil {
		t.Fatalf("maxDepth=50 should allow this chain, got: %v", err)
	}
}

// TestMaxDepth_CycleTakesPrecedence verifies that cycle detection runs
// BEFORE depth checking, so a circular dependency always reports as a cycle
// even when maxDepth is very low.
func TestMaxDepth_CycleTakesPrecedence(t *testing.T) {
	type SvcA struct{}
	type SvcB struct{}

	c := di.New().WithMaxDepth(2) // Low limit, but enough to let cycle detection fire first
	_ = di.Provide[*SvcA](c, func(c *di.Container) (*SvcA, error) {
		_, _ = di.Invoke[*SvcB](c)
		return &SvcA{}, nil
	})
	_ = di.Provide[*SvcB](c, func(c *di.Container) (*SvcB, error) {
		_, _ = di.Invoke[*SvcA](c)
		return &SvcB{}, nil
	})

	var panicVal any
	func() {
		defer func() { panicVal = recover() }()
		di.MustInvoke[*SvcA](c)
	}()

	err, ok := panicVal.(error)
	if !ok {
		t.Fatalf("expected error panic, got %T: %v", panicVal, panicVal)
	}
	// MUST be ErrCyclicDependency, NOT ErrMaxDepthExceeded
	if !errors.Is(err, di.ErrCyclicDependency) {
		t.Fatalf("expected ErrCyclicDependency (cycle takes precedence), got: %v", err)
	}
	if errors.Is(err, di.ErrMaxDepthExceeded) {
		t.Fatalf("should NOT be ErrMaxDepthExceeded when cycle exists, got: %v", err)
	}
}
