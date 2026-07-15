package di

import (
	"errors"
	"sync"
	"testing"
)

// ─── test provider & factory ──────────────────────────────────────────────────

type testSender interface {
	Send(to, msg string) error
}

type aliyunFactory struct{}

func (f *aliyunFactory) Code() string                    { return "aliyun" }
func (f *aliyunFactory) New(_ any, _ any) (testSender, error) {
	return &mockSender{name: "aliyun"}, nil
}

type wechatFactory struct{}

func (f *wechatFactory) Code() string                    { return "wechat" }
func (f *wechatFactory) New(_ any, _ any) (testSender, error) {
	return &mockSender{name: "wechat"}, nil
}

type mockSender struct {
	name string
}

func (m *mockSender) Send(to, msg string) error { return nil }

type failingFactory struct{ code string }

func (f *failingFactory) Code() string                    { return f.code }
func (f *failingFactory) New(_ any, _ any) (testSender, error) {
	return nil, errors.New("factory error")
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestProviderRegistry_Register(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	if r.Len() != 0 {
		t.Fatalf("expected 0, got %d", r.Len())
	}

	if err := r.Register(&aliyunFactory{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if r.Len() != 1 {
		t.Fatalf("expected 1, got %d", r.Len())
	}

	// Duplicate
	if err := r.Register(&aliyunFactory{}); err == nil {
		t.Fatal("expected error on duplicate")
	}
}

func TestProviderRegistry_RegisterNil(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	if err := r.Register(nil); err == nil {
		t.Fatal("expected error on nil factory")
	}
}

func TestProviderRegistry_RegisterEmptyCode(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	f := &mockFactory{code: ""}
	if err := r.Register(f); err == nil {
		t.Fatal("expected error on empty code")
	}
}

func TestProviderRegistry_Get(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})

	f, ok := r.Get("aliyun")
	if !ok {
		t.Fatal("expected aliyun to exist")
	}
	if f.Code() != "aliyun" {
		t.Fatalf("expected aliyun, got %s", f.Code())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent code")
	}
}

func TestProviderRegistry_List(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})
	r.MustRegister(&wechatFactory{})

	list := r.List()
	if len(list) != 2 || list[0] != "aliyun" || list[1] != "wechat" {
		t.Fatalf("unexpected list: %v", list)
	}
}

func TestProviderRegistry_Create(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})

	sender, err := r.Create(nil, "aliyun", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sender == nil {
		t.Fatal("expected non-nil sender")
	}

	_, err = r.Create(nil, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error on nonexistent")
	}
}

func TestProviderRegistry_MustCreatePanic(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	r.MustCreate(nil, "missing", nil)
}

func TestProviderRegistry_CreateFactoryError(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&failingFactory{code: "fail"})

	_, err := r.Create(nil, "fail", nil)
	if err == nil {
		t.Fatal("expected factory error")
	}
}

func TestProviderRegistry_ForEach(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})
	r.MustRegister(&wechatFactory{})

	var codes []string
	r.ForEach(func(code string, _ ProviderFactory[testSender]) error {
		codes = append(codes, code)
		return nil
	})
	if len(codes) != 2 || codes[0] != "aliyun" || codes[1] != "wechat" {
		t.Fatalf("unexpected ForEach codes: %v", codes)
	}
}

func TestProviderRegistry_ForEachEarlyExit(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})
	r.MustRegister(&wechatFactory{})

	sentinel := errors.New("stop")
	var count int
	err := r.ForEach(func(code string, _ ProviderFactory[testSender]) error {
		count++
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 iteration, got %d", count)
	}
}

func TestProviderRegistry_Reset(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})
	r.Reset()
	if r.Len() != 0 {
		t.Fatalf("expected 0 after Reset, got %d", r.Len())
	}
}

func TestProviderRegistry_Concurrency(t *testing.T) {
	r := NewProviderRegistry[testSender]()
	r.MustRegister(&aliyunFactory{})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _ := r.Create(nil, "aliyun", nil)
			if s == nil {
				t.Error("sender is nil")
			}
		}()
	}
	wg.Wait()
}

// mockFactory for edge cases
type mockFactory struct {
	code string
	new  func(ctx any, cfg any) (testSender, error)
}

func (f *mockFactory) Code() string                         { return f.code }
func (f *mockFactory) New(ctx any, cfg any) (testSender, error) { return f.new(ctx, cfg) }
