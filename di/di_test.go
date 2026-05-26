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
