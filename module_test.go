package astra_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── Module interface helpers ────────────────────────────────────────────────

type stubModule struct {
	name      string
	installFn func(*astra.App) error
}

func (s *stubModule) Name() string { return s.name }
func (s *stubModule) Install(app *astra.App) error {
	if s.installFn != nil {
		return s.installFn(app)
	}
	return nil
}

func stubMod(name string) *stubModule { return &stubModule{name: name} }

// ─── App.Register — basic wiring ─────────────────────────────────────────────

func TestRegister_InstallIsCalledOnce(t *testing.T) {
	count := 0
	m := &stubModule{
		name: "counter",
		installFn: func(_ *astra.App) error {
			count++
			return nil
		},
	}
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(m))
	if count != 1 {
		t.Errorf("expected Install to be called once, got %d", count)
	}
}

func TestRegister_MultipleModules_AllInstalled(t *testing.T) {
	installed := map[string]bool{}
	makeM := func(name string) astra.Module {
		return astra.NewModuleFunc(name, func(_ *astra.App) error {
			installed[name] = true
			return nil
		})
	}
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(makeM("a"), makeM("b"), makeM("c")))
	for _, name := range []string{"a", "b", "c"} {
		if !installed[name] {
			t.Errorf("module %q was not installed", name)
		}
	}
}

func TestRegister_InstallError_StopsEarly(t *testing.T) {
	installed := []string{}
	errBoom := errors.New("boom")

	app := testutil.NewTestApp()
	err := app.Register(
		astra.NewModuleFunc("ok1", func(_ *astra.App) error {
			installed = append(installed, "ok1")
			return nil
		}),
		astra.NewModuleFunc("fail", func(_ *astra.App) error {
			return errBoom
		}),
		astra.NewModuleFunc("ok2", func(_ *astra.App) error {
			installed = append(installed, "ok2")
			return nil
		}),
	)
	if err == nil {
		t.Fatal("expected error from failed module")
	}
	if !errors.Is(err, errBoom) {
		t.Errorf("error should wrap original: %v", err)
	}
	if len(installed) != 1 || installed[0] != "ok1" {
		t.Errorf("only ok1 should be installed before the failure, got %v", installed)
	}
}

func TestRegister_InstallError_WrapsModuleName(t *testing.T) {
	app := testutil.NewTestApp()
	err := app.Register(astra.NewModuleFunc("bad-module", func(_ *astra.App) error {
		return errors.New("database not reachable")
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	// Error message must contain the module name.
	if !containsStr(err.Error(), "bad-module") {
		t.Errorf("error should contain module name: %v", err)
	}
}

// ─── Duplicate detection ──────────────────────────────────────────────────────

func TestRegister_DuplicateName_ReturnsError(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubMod("db")))
	err := app.Register(stubMod("db"))
	if err == nil {
		t.Fatal("expected error when registering duplicate module name")
	}
	if !containsStr(err.Error(), "db") {
		t.Errorf("error should mention the duplicate name: %v", err)
	}
}

func TestRegister_DuplicateInSameCall_ReturnsError(t *testing.T) {
	app := testutil.NewTestApp()
	err := app.Register(stubMod("x"), stubMod("x"))
	if err == nil {
		t.Fatal("expected error for duplicate name in the same Register call")
	}
}

// ─── Modules / HasModule ──────────────────────────────────────────────────────

func TestModules_ReturnsInstalledModules(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubMod("alpha"), stubMod("beta")))

	mods := app.Modules()
	if len(mods) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(mods))
	}
	if mods["alpha"] == nil {
		t.Error("module alpha missing from Modules()")
	}
	if mods["beta"] == nil {
		t.Error("module beta missing from Modules()")
	}
}

func TestModules_ReturnsCopy(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubMod("m1")))

	// Mutating the returned map must not affect the App's internal state.
	mods := app.Modules()
	delete(mods, "m1")

	if !app.HasModule("m1") {
		t.Error("mutating the returned map must not affect app state")
	}
}

func TestHasModule_TrueForInstalled(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubMod("health")))
	if !app.HasModule("health") {
		t.Error("HasModule should return true for installed module")
	}
}

func TestHasModule_FalseForUnknown(t *testing.T) {
	app := testutil.NewTestApp()
	if app.HasModule("nonexistent") {
		t.Error("HasModule should return false for unknown module")
	}
}

// ─── Module can wire routes and middleware ────────────────────────────────────

func TestRegister_ModuleRegistersRoutes(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.NewModuleFunc("api", func(app *astra.App) error {
		app.GET("/ping", func(c *astra.Ctx) error {
			return c.JSON(200, astra.Map{"ok": true})
		})
		return nil
	})))

	s := testutil.NewServer(t, app)
	s.GET("/ping").AssertStatus(200)
}

func TestRegister_ModuleRegistersMiddleware(t *testing.T) {
	called := false
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.NewModuleFunc("mw", func(app *astra.App) error {
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
		t.Error("module middleware was not executed")
	}
}

func TestRegister_ModuleRegistersLifecycleHooks(t *testing.T) {
	app := testutil.NewTestApp()
	hookCalled := false
	testutil.AssertNoError(t, app.Register(astra.NewModuleFunc("hooks", func(app *astra.App) error {
		app.OnStart(func(_ context.Context) error {
			hookCalled = true
			return nil
		})
		return nil
	})))
	// Hook registration must not error; the hook itself runs at server start.
	_ = hookCalled
}

// ─── Module can nest sub-modules ─────────────────────────────────────────────

func TestRegister_NestedModule(t *testing.T) {
	inner := stubMod("inner")
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.NewModuleFunc("outer", func(app *astra.App) error {
		return app.Register(inner)
	})))
	if !app.HasModule("inner") {
		t.Error("nested module should be visible via HasModule")
	}
	if !app.HasModule("outer") {
		t.Error("outer module should be visible via HasModule")
	}
}

// ─── ModuleFunc ───────────────────────────────────────────────────────────────

func TestModuleFunc_NameAndInstall(t *testing.T) {
	m := astra.NewModuleFunc("test-func-module", func(_ *astra.App) error { return nil })
	testutil.AssertEqual(t, "test-func-module", m.Name())

	app := testutil.NewTestApp()
	testutil.AssertNoError(t, m.Install(app))
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

// ─── Plugin / PluginAsModule ─────────────────────────────────────────────────

type stubPlugin struct {
	name   string
	initFn func(*astra.App) error
}

func (s *stubPlugin) Name() string { return s.name }
func (s *stubPlugin) Init(app *astra.App) error {
	if s.initFn != nil {
		return s.initFn(app)
	}
	return nil
}

func stubPlug(name string) *stubPlugin { return &stubPlugin{name: name} }

func TestRegisterPlugin_InitIsCalledOnce(t *testing.T) {
	count := 0
	p := &stubPlugin{
		name: "counter-plugin",
		initFn: func(_ *astra.App) error {
			count++
			return nil
		},
	}
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.RegisterPlugin(p))
	if count != 1 {
		t.Errorf("expected Init to be called once, got %d", count)
	}
}

func TestRegisterPlugin_DuplicateName_ReturnsError(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.RegisterPlugin(stubPlug("swagger")))
	err := app.RegisterPlugin(stubPlug("swagger"))
	if err == nil {
		t.Fatal("expected error when registering duplicate plugin name")
	}
	if !containsStr(err.Error(), "swagger") {
		t.Errorf("error should mention the duplicate name: %v", err)
	}
}

func TestRegisterPlugin_PluginAndModule_SharedNamespace(t *testing.T) {
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(stubMod("shared")))
	err := app.RegisterPlugin(stubPlug("shared"))
	if err == nil {
		t.Fatal("plugin and module must share the same name namespace")
	}
}

func TestPluginAsModule_WrapsCorrectly(t *testing.T) {
	called := false
	p := &stubPlugin{
		name: "wrapped",
		initFn: func(_ *astra.App) error {
			called = true
			return nil
		},
	}
	app := testutil.NewTestApp()
	testutil.AssertNoError(t, app.Register(astra.PluginAsModule(p)))
	if !called {
		t.Error("PluginAsModule: Init was not called through Install")
	}
	if !app.HasModule("wrapped") {
		t.Error("PluginAsModule: module should be visible via HasModule")
	}
}
