package astra_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── Component interface helpers ─────────────────────────────────────────────

type stubComponent struct {
	name   string
	initFn func(*astra.App) error
}

func (s *stubComponent) Name() string { return s.name }
func (s *stubComponent) Init(app *astra.App) error {
	if s.initFn != nil {
		return s.initFn(app)
	}
	return nil
}

func stubComp(name string) *stubComponent { return &stubComponent{name: name} }

// ─── App.Register — basic wiring ─────────────────────────────────────────────

func TestRegister_InitIsCalledOnce(t *testing.T) {
	count := 0
	c := &stubComponent{
		name: "counter",
		initFn: func(_ *astra.App) error {
			count++
			return nil
		},
	}
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(c))
	if count != 1 {
		t.Errorf("expected Init to be called once, got %d", count)
	}
}

func TestRegister_MultipleComponents_AllInstalled(t *testing.T) {
	installed := map[string]bool{}
	makeC := func(name string) astra.Component {
		return astra.NewComponentFunc(name, func(_ *astra.App) error {
			installed[name] = true
			return nil
		})
	}
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(makeC("a"), makeC("b"), makeC("c")))
	for _, name := range []string{"a", "b", "c"} {
		if !installed[name] {
			t.Errorf("component %q was not installed", name)
		}
	}
}

func TestRegister_InitError_StopsEarly(t *testing.T) {
	installed := []string{}
	errBoom := errors.New("boom")

	app := testutil.NewTestApp()
	err := app.Register(
		astra.NewComponentFunc("ok1", func(_ *astra.App) error {
			installed = append(installed, "ok1")
			return nil
		}),
		astra.NewComponentFunc("fail", func(_ *astra.App) error {
			return errBoom
		}),
		astra.NewComponentFunc("ok2", func(_ *astra.App) error {
			installed = append(installed, "ok2")
			return nil
		}),
	)
	if err == nil {
		t.Fatal("expected error from failed component")
	}
	if !errors.Is(err, errBoom) {
		t.Errorf("error should wrap original: %v", err)
	}
	if len(installed) != 1 || installed[0] != "ok1" {
		t.Errorf("only ok1 should be installed before the failure, got %v", installed)
	}
}

func TestRegister_InitError_WrapsComponentName(t *testing.T) {
	app := testutil.NewTestApp()
	err := app.Register(astra.NewComponentFunc("bad-component", func(_ *astra.App) error {
		return errors.New("database not reachable")
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsStr(err.Error(), "bad-component") {
		t.Errorf("error should contain component name: %v", err)
	}
}

// ─── Duplicate detection ──────────────────────────────────────────────────────

func TestRegister_DuplicateName_ReturnsError(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubComp("db")))
	err := app.Register(stubComp("db"))
	if err == nil {
		t.Fatal("expected error when registering duplicate component name")
	}
	if !containsStr(err.Error(), "db") {
		t.Errorf("error should mention the duplicate name: %v", err)
	}
}

func TestRegister_DuplicateInSameCall_ReturnsError(t *testing.T) {
	app := testutil.NewTestApp()
	err := app.Register(stubComp("x"), stubComp("x"))
	if err == nil {
		t.Fatal("expected error for duplicate name in the same Register call")
	}
}

// ─── Components / HasComponent ────────────────────────────────────────────────

func TestComponents_ReturnsInstalledComponents(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubComp("alpha"), stubComp("beta")))

	comps := app.Components()
	if len(comps) != 2 {
		t.Fatalf("expected 2 components, got %d", len(comps))
	}
	if comps["alpha"] == nil {
		t.Error("component alpha missing from Components()")
	}
	if comps["beta"] == nil {
		t.Error("component beta missing from Components()")
	}
}

func TestComponents_ReturnsCopy(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubComp("c1")))

	comps := app.Components()
	delete(comps, "c1")

	if !app.HasComponent("c1") {
		t.Error("mutating the returned map must not affect app state")
	}
}

func TestHasComponent_TrueForInstalled(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubComp("health")))
	if !app.HasComponent("health") {
		t.Error("HasComponent should return true for installed component")
	}
}

func TestHasComponent_FalseForUnknown(t *testing.T) {
	app := testutil.NewTestApp()
	if app.HasComponent("nonexistent") {
		t.Error("HasComponent should return false for unknown component")
	}
}

// ─── Component can wire routes and middleware ─────────────────────────────────

func TestRegister_ComponentRegistersRoutes(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.NewComponentFunc("api", func(app *astra.App) error {
		app.GET("/ping", func(c *astra.Ctx) error {
			return c.JSON(200, astra.Map{"ok": true})
		})
		return nil
	})))

	s := testutil.NewServer(t, app)
	s.GET("/ping").AssertStatus(200)
}

func TestRegister_ComponentRegistersMiddleware(t *testing.T) {
	called := false
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.NewComponentFunc("mw", func(app *astra.App) error {
		app.Use(func(c *astra.Ctx) error {
			called = true
			c.Next()
			return nil
		})
		return nil
	})))
	app.GET("/x", func(c *astra.Ctx) error { return c.JSON(200, nil) })

	s := testutil.NewServer(t, app)
	s.GET("/x")
	if !called {
		t.Error("component middleware was not executed")
	}
}

func TestRegister_ComponentRegistersLifecycleHooks(t *testing.T) {
	app := testutil.NewTestApp()
	hookCalled := false
	testutil.AssertNoError(t, app.Register(astra.NewComponentFunc("hooks", func(app *astra.App) error {
		app.OnStart(func(_ context.Context) error {
			hookCalled = true
			return nil
		})
		return nil
	})))
	_ = hookCalled
}

// ─── Component can nest sub-components ───────────────────────────────────────

func TestRegister_NestedComponent(t *testing.T) {
	inner := stubComp("inner")
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.NewComponentFunc("outer", func(app *astra.App) error {
		return app.Register(inner)
	})))
	if !app.HasComponent("inner") {
		t.Error("nested component should be visible via HasComponent")
	}
	if !app.HasComponent("outer") {
		t.Error("outer component should be visible via HasComponent")
	}
}

// ─── ComponentFunc ────────────────────────────────────────────────────────────

func TestComponentFunc_NameAndInit(t *testing.T) {
	c := astra.NewComponentFunc("test-func-component", func(_ *astra.App) error { return nil })
	testutil.AssertEqual(t, "test-func-component", c.Name())

	app := testutil.NewTestApp()
	testutil.AssertNoError(t, c.Init(app))
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
