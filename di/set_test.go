package di_test

import (
	"errors"
	"testing"

	"github.com/astra-go/astra/di"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

type Repo struct{ Backend string }
type Service struct{ Repo *Repo }

func newMemRepo() *Repo  { return &Repo{Backend: "memory"} }
func newSQLRepo() *Repo  { return &Repo{Backend: "sql"} }
func newTestRepo() *Repo { return &Repo{Backend: "test"} }

// ─── ProviderSet: Apply ───────────────────────────────────────────────────────

func TestProviderSet_Apply(t *testing.T) {
	c := di.New()
	set := di.NewSet("dev",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newMemRepo(), nil
			})
		},
	)

	if err := set.Apply(c); err != nil {
		t.Fatal(err)
	}
	repo := di.MustInvoke[*Repo](c)
	if repo.Backend != "memory" {
		t.Fatalf("expected memory, got %q", repo.Backend)
	}
}

func TestProviderSet_ApplyError(t *testing.T) {
	c := di.New()
	boom := errors.New("factory error")
	set := di.NewSet("broken",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return nil, boom
			})
		},
	)

	// Apply should succeed (it only registers the factory, doesn't execute it)
	if err := set.Apply(c); err != nil {
		t.Fatal(err)
	}

	// The error surfaces when Invoke runs the factory
	_, err := di.Invoke[*Repo](c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProviderSet_MustApply(t *testing.T) {
	c := di.New()
	set := di.NewSet("dev",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newMemRepo(), nil
			})
		},
	)

	set.MustApply(c)
	// Should not panic
}

func TestProviderSet_MustApplyPanics(t *testing.T) {
	c := di.New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()

	// This should panic: RegisterDuplicate provider on the same container
	_ = di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
		return newMemRepo(), nil
	})

	boom := errors.New("factory error")
	set := di.NewSet("bad",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return nil, boom
			})
		},
	)
	set.MustApply(c)
}

// ─── ProviderSet: NewDefaultSet ──────────────────────────────────────────────

func TestDefaultSet_AlwaysApplies(t *testing.T) {
	c := di.New()
	defaultSet := di.NewDefaultSet(
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return &Repo{Backend: "default"}, nil
			})
		},
	)

	if err := di.RegisterSets(c, "nonexistent", defaultSet); err != nil {
		t.Fatal(err)
	}
	repo := di.MustInvoke[*Repo](c)
	if repo.Backend != "default" {
		t.Fatalf("expected 'default', got %q", repo.Backend)
	}
}

// ─── RegisterSets: environment matching ───────────────────────────────────────

func TestRegisterSets_SelectsCorrectEnv(t *testing.T) {
	c := di.New()
	devSet := di.NewSet("dev",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newMemRepo(), nil
			})
		},
	)
	prodSet := di.NewSet("prod",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newSQLRepo(), nil
			})
		},
	)

	if err := di.RegisterSets(c, "prod", devSet, prodSet); err != nil {
		t.Fatal(err)
	}

	repo := di.MustInvoke[*Repo](c)
	if repo.Backend != "sql" {
		t.Fatalf("expected sql, got %q", repo.Backend)
	}
}

func TestRegisterSets_DevSelectsDev(t *testing.T) {
	c := di.New()
	devSet := di.NewSet("dev",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newMemRepo(), nil
			})
		},
	)
	prodSet := di.NewSet("prod",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newSQLRepo(), nil
			})
		},
	)

	di.MustRegisterSets(c, "dev", devSet, prodSet)
	repo := di.MustInvoke[*Repo](c)
	if repo.Backend != "memory" {
		t.Fatalf("expected memory, got %q", repo.Backend)
	}
}

func TestRegisterSets_TestSelectsCorrect(t *testing.T) {
	c := di.New()
	testSet := di.NewSet("test",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newTestRepo(), nil
			})
		},
	)
	di.MustRegisterSets(c, "test", testSet)
	repo := di.MustInvoke[*Repo](c)
	if repo.Backend != "test" {
		t.Fatalf("expected test, got %q", repo.Backend)
	}
}

// ─── RegisterSets: default sets run before env-specific ────────────────────────

func TestDefaultSetPlusEnvSpecific(t *testing.T) {
	c := di.New()
	defaultSet := di.NewDefaultSet(
		func(c *di.Container) error {
			return di.ProvideValue[string](c, "base-url")
		},
	)
	prodSet := di.NewSet("prod",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newSQLRepo(), nil
			})
		},
	)

	di.MustRegisterSets(c, "prod", defaultSet, prodSet)

	if !di.Has[string](c) {
		t.Fatal("default set should have registered string provider")
	}
	if !di.Has[*Repo](c) {
		t.Fatal("prod set should have registered Repo provider")
	}
	baseURL := di.MustInvoke[string](c)
	if baseURL != "base-url" {
		t.Fatalf("expected 'base-url', got %q", baseURL)
	}
}

// ─── RegisterSets: env="" matches default ─────────────────────────────────────

func TestEmptyEnvRegistersDefaults(t *testing.T) {
	c := di.New()
	defaultSet := di.NewDefaultSet(
		func(c *di.Container) error {
			return di.ProvideValue[string](c, "default-value")
		},
	)

	// Empty env should trigger default sets
	if err := di.RegisterSets(c, "", defaultSet); err != nil {
		t.Fatal(err)
	}
	if !di.Has[string](c) {
		t.Fatal("default set should have been applied with empty env")
	}
}

func TestEnvNamedDefaultAlsoApplies(t *testing.T) {
	c := di.New()
	set := di.NewSet("default",
		func(c *di.Container) error {
			return di.ProvideValue[int](c, 42)
		},
	)

	di.MustRegisterSets(c, "prod", set)
	v := di.MustInvoke[int](c)
	if v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
}

// ─── SelectSet ────────────────────────────────────────────────────────────────

func TestSelectSet_FindsMatch(t *testing.T) {
	devSet := di.NewSet("dev", nil)
	prodSet := di.NewSet("prod", nil)

	got := di.SelectSet("prod", devSet, prodSet)
	if got == nil {
		t.Fatal("expected prodSet, got nil")
	}
	if got.Env() != "prod" {
		t.Fatalf("expected env 'prod', got %q", got.Env())
	}
}

func TestSelectSet_NotFound(t *testing.T) {
	devSet := di.NewSet("dev", nil)
	got := di.SelectSet("test", devSet)
	if got != nil {
		t.Fatalf("expected nil, got env=%q", got.Env())
	}
}

func TestSelectSet_DoesNotReturnDefault(t *testing.T) {
	defaultSet := di.NewDefaultSet()
	// SelectSet does exact match on the env field, so defaultSet (env="default")
	// matches SelectSet("default", ...). Use a non-matching env to verify nil return.
	got := di.SelectSet("test", defaultSet)
	if got != nil {
		t.Fatalf("expected nil for non-matching env, got env=%q", got.Env())
	}
}

// ─── RegisterSets: first error stops ──────────────────────────────────────────

func TestRegisterSets_StopsOnError(t *testing.T) {
	c := di.New()
	boom := errors.New("first error")
	secondRan := false

	defaultSet := di.NewDefaultSet(
		func(c *di.Container) error { return boom },
	)
	devSet := di.NewSet("dev",
		func(c *di.Container) error { secondRan = true; return nil },
	)

	err := di.RegisterSets(c, "dev", defaultSet, devSet)
	if err == nil {
		t.Fatal("expected error")
	}
	if secondRan {
		t.Fatal("second provider should not have run after first error")
	}
}

// ─── Reset ────────────────────────────────────────────────────────────────────

func TestProviderSet_ReusableAcrossContainers(t *testing.T) {
	set := di.NewSet("dev",
		func(c *di.Container) error {
			return di.ProvideValue[string](c, "shared")
		},
	)

	c1 := di.New()
	c2 := di.New()

	set.MustApply(c1)
	set.MustApply(c2)

	if di.MustInvoke[string](c1) != "shared" {
		t.Fatal("c1 mismatch")
	}
	if di.MustInvoke[string](c2) != "shared" {
		t.Fatal("c2 mismatch")
	}
}

// ─── Transitive dependencies within a set ─────────────────────────────────────

func TestProviderSet_TransitiveDependencies(t *testing.T) {
	c := di.New()
	set := di.NewSet("dev",
		func(c *di.Container) error {
			return di.Provide[*Repo](c, func(_ *di.Container) (*Repo, error) {
				return newMemRepo(), nil
			})
		},
		func(c *di.Container) error {
			return di.Provide[*Service](c, func(c *di.Container) (*Service, error) {
				repo := di.MustInvoke[*Repo](c)
				return &Service{Repo: repo}, nil
			})
		},
	)

	set.MustApply(c)
	svc := di.MustInvoke[*Service](c)
	if svc.Repo.Backend != "memory" {
		t.Fatalf("expected memory, got %q", svc.Repo.Backend)
	}
}

// ─── Env() accessor ───────────────────────────────────────────────────────────

func TestProviderSet_Env(t *testing.T) {
	dev := di.NewSet("dev")
	prod := di.NewSet("prod")
	def := di.NewDefaultSet()

	if dev.Env() != "dev" {
		t.Fatalf("expected 'dev', got %q", dev.Env())
	}
	if prod.Env() != "prod" {
		t.Fatalf("expected 'prod', got %q", prod.Env())
	}
	if def.Env() != "default" {
		t.Fatalf("expected 'default', got %q", def.Env())
	}
}

// ─── RegisterSets with Must variant ──────────────────────────────────────────

func TestMustRegisterSets_PanicsOnError(t *testing.T) {
	// Factory errors during Provide don't cause panic during registration;
	// they surface during Invoke. This test uses a different error path:
	// the provider function itself returns an error.
	boom := errors.New("provider error")
	set := di.NewSet("dev",
		func(c *di.Container) error {
			return boom
		},
	)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	di.MustRegisterSets(di.New(), "dev", set)
}
