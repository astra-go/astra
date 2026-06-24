package backend

import (
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

type mockRepo struct {
	name string
}

func (m *mockRepo) Name() string { return m.name }

// ─── New & defaults ───────────────────────────────────────────────────────────

func TestNew_DefaultMapping(t *testing.T) {
	s := New("test-svc")
	m := s.Mapping()
	tests := []struct {
		env      string
		expected string
	}{
		{"dev", "memory"},
		{"test", "memory"},
		{"prod", "sql-redis"},
		{"staging", "sql-redis"},
	}
	for _, tt := range tests {
		got, ok := m[tt.env]
		if !ok {
			t.Errorf("missing mapping for env %q", tt.env)
			continue
		}
		if got != tt.expected {
			t.Errorf("mapping[%q] = %q, want %q", tt.env, got, tt.expected)
		}
	}
}

func TestNew_Name(t *testing.T) {
	s := New("my-svc")
	if got := s.Name(); got != "my-svc" {
		t.Errorf("Name() = %q, want %q", got, "my-svc")
	}
}

// ─── Register / MustRegister ─────────────────────────────────────────────────

func TestRegister(t *testing.T) {
	s := New("test")
	s.Register("memory", &mockRepo{name: "mem"})
	if !s.HasBackend("memory") {
		t.Error("expected memory to be registered")
	}
}

func TestRegister_PanicsOnDuplicate(t *testing.T) {
	s := New("test")
	s.Register("memory", &mockRepo{name: "mem"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	s.Register("memory", &mockRepo{name: "mem2"})
}

func TestMustRegister_Overwrites(t *testing.T) {
	s := New("test")
	s.MustRegister("memory", &mockRepo{name: "mem"})
	s.MustRegister("memory", &mockRepo{name: "mem2"}) // no panic

	impl, ok := s.Select("dev")
	if !ok {
		t.Fatal("expected to find backend for dev")
	}
	if r, ok := impl.(*mockRepo); !ok || r.Name() != "mem2" {
		t.Errorf("got %v, want mem2", r)
	}
}

// ─── Select / MustSelect ─────────────────────────────────────────────────────

func TestSelect_Found(t *testing.T) {
	s := New("test")
	repo := &mockRepo{name: "my-repo"}
	s.Register("memory", repo)

	impl, ok := s.Select("dev")
	if !ok {
		t.Fatal("Select(dev) should find memory backend")
	}
	if impl != repo {
		t.Errorf("Select returned %v, want %v", impl, repo)
	}
}

func TestSelect_NotFound_NoMapping(t *testing.T) {
	s := New("test")
	s.Register("memory", &mockRepo{name: "mem"})

	// "ci" is not in the default mapping
	impl, ok := s.Select("ci")
	if ok {
		t.Errorf("expected false for unknown env, got impl=%v", impl)
	}
}

func TestSelect_NotFound_NotRegistered(t *testing.T) {
	s := New("test")
	// Only "sql-redis" is mapped for prod, but we haven't registered it
	impl, ok := s.Select("prod")
	if ok {
		t.Errorf("expected false for unregistered backend, got impl=%v", impl)
	}
}

func TestMustSelect_Found(t *testing.T) {
	s := New("test")
	repo := &mockRepo{name: "prod-repo"}
	s.Register("sql-redis", repo)

	got := s.MustSelect("prod")
	if got != repo {
		t.Errorf("MustSelect(prod) = %v, want %v", got, repo)
	}
}

func TestMustSelect_PanicsOnMissing(t *testing.T) {
	s := New("test")
	// no backends registered at all

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on missing backend")
		}
	}()
	s.MustSelect("prod")
}

// ─── Force ────────────────────────────────────────────────────────────────────

func TestForce_Override(t *testing.T) {
	s := New("test")
	mem := &mockRepo{name: "memory"}
	sql := &mockRepo{name: "sql-redis"}
	s.Register("memory", mem)
	s.Register("sql-redis", sql)

	// Force sql-redis regardless of env
	s.Force("sql-redis")

	impl, ok := s.Select("dev") // dev → memory normally
	if !ok {
		t.Fatal("expected to find forced backend")
	}
	if impl != sql {
		t.Errorf("Force: got %v, want %v", impl, sql)
	}
}

func TestForce_Clear(t *testing.T) {
	s := New("test")
	mem := &mockRepo{name: "memory"}
	sql := &mockRepo{name: "sql-redis"}
	s.Register("memory", mem)
	s.Register("sql-redis", sql)

	s.Force("sql-redis")
	s.Force("") // clear

	impl, ok := s.Select("dev")
	if !ok {
		t.Fatal("expected dev→memory after clearing force")
	}
	if impl != mem {
		t.Errorf("after Force(''), got %v, want %v", impl, mem)
	}
}

// ─── WithBackend option ───────────────────────────────────────────────────────

func TestWithBackend_Option(t *testing.T) {
	mem := &mockRepo{name: "memory"}
	s := New("test", WithBackend("memory"))
	s.Register("memory", mem)

	// Even calling Select("prod") should return "memory"
	impl, ok := s.Select("prod")
	if !ok {
		t.Fatal("WithBackend: Select should find forced backend")
	}
	if impl != mem {
		t.Errorf("WithBackend: got %v, want %v", impl, mem)
	}
}

func TestWithBackend_Explicit(t *testing.T) {
	s := New("test", WithBackend("my-backend"))
	name, ok := s.Explicit()
	if !ok {
		t.Fatal("expected explicit backend to be set")
	}
	if name != "my-backend" {
		t.Errorf("Explicit() = %q, want %q", name, "my-backend")
	}
}

// ─── WithMapping option ───────────────────────────────────────────────────────

func TestWithMapping_Overrides(t *testing.T) {
	s := New("test", WithMapping(map[string]string{
		"dev":  "sql-redis",
		"ci":   "memory",
	}))
	m := s.Mapping()
	if m["dev"] != "sql-redis" {
		t.Errorf("WithMapping(dev→sql-redis) overwrite failed, got %q", m["dev"])
	}
	if m["ci"] != "memory" {
		t.Errorf("WithMapping(ci→memory) new mapping failed, got %q", m["ci"])
	}
	// Defaults for other envs should still exist
	if m["prod"] != "sql-redis" {
		t.Errorf("default prod mapping lost after WithMapping, got %q", m["prod"])
	}
}

// ─── SetMapping ───────────────────────────────────────────────────────────────

func TestSetMapping(t *testing.T) {
	s := New("test")
	s.SetMapping("test", "sql-redis")
	m := s.Mapping()
	if m["test"] != "sql-redis" {
		t.Errorf("SetMapping(test→sql-redis): got %q", m["test"])
	}
}

// ─── Available / HasBackend / Count ───────────────────────────────────────────

func TestAvailable(t *testing.T) {
	s := New("test")
	s.Register("mem", &mockRepo{name: "mem"})
	s.Register("sql", &mockRepo{name: "sql"})

	avail := s.Available()
	if len(avail) != 2 {
		t.Errorf("Available() len = %d, want 2", len(avail))
	}
}

func TestHasBackend(t *testing.T) {
	s := New("test")
	s.Register("memory", &mockRepo{name: "mem"})
	if !s.HasBackend("memory") {
		t.Error("HasBackend(memory) should be true")
	}
	if s.HasBackend("nonexistent") {
		t.Error("HasBackend(nonexistent) should be false")
	}
}

func TestCount(t *testing.T) {
	s := New("test")
	if n := s.Count(); n != 0 {
		t.Errorf("empty selector Count() = %d, want 0", n)
	}
	s.Register("a", &mockRepo{name: "a"})
	s.Register("b", &mockRepo{name: "b"})
	if n := s.Count(); n != 2 {
		t.Errorf("Count() = %d, want 2", n)
	}
}

// ─── Thread safety (race detector) ────────────────────────────────────────────

func TestConcurrentAccess(t *testing.T) {
	s := New("test")
	s.Register("memory", &mockRepo{name: "mem"})
	s.Register("sql-redis", &mockRepo{name: "sql"})

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			s.Select("dev")
			s.Select("prod")
			s.Available()
			s.Count()
			s.Mapping()
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 50; i++ {
			s.Force("memory")
			s.Force("")
			s.SetMapping("dev", "sql-redis")
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

// ─── Edge cases ───────────────────────────────────────────────────────────────

func TestSelect_UnknownEnvNoBackends(t *testing.T) {
	s := New("test", WithMapping(map[string]string{"dev": "memory"}))
	// nothing registered
	impl, ok := s.Select("dev")
	if ok {
		t.Errorf("expected false when backend not registered, got %v", impl)
	}
}

func TestSelect_ExplicitUnregistered(t *testing.T) {
	s := New("test", WithBackend("nonexistent"))
	impl, ok := s.Select("dev")
	if ok {
		t.Errorf("expected false when forced backend not registered, got %v", impl)
	}
}

func TestRegister_SameNameDifferentSelector(t *testing.T) {
	a := New("a")
	b := New("b")
	a.Register("memory", &mockRepo{name: "a-mem"})
	b.Register("memory", &mockRepo{name: "b-mem"}) // same name, different instance — no panic
	a.MustSelect("dev")
	b.MustSelect("dev")
}
