package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/astra-go/astra/testutil"
)

// ─── Test fixtures ────────────────────────────────────────────────────────

// SMSSender is a test provider interface.
type SMSSender interface {
	Send(to, msg string) string
}

type smsConfig struct {
	APIKey string
}

type aliyunSMS struct{ apiKey string }

func (s *aliyunSMS) Send(to, msg string) string {
	return fmt.Sprintf("aliyun:%s:%s", to, msg)
}

type aliyunFactory struct{}

func (f *aliyunFactory) Code() string { return "aliyun" }
func (f *aliyunFactory) New(ctx any, cfg any) (SMSSender, error) {
	c, ok := cfg.(smsConfig)
	if !ok {
		return nil, fmt.Errorf("expected smsConfig, got %T", cfg)
	}
	return &aliyunSMS{apiKey: c.APIKey}, nil
}

type tencentSMS struct{ apiKey string }

func (s *tencentSMS) Send(to, msg string) string {
	return fmt.Sprintf("tencent:%s:%s", to, msg)
}

type tencentFactory struct{}

func (f *tencentFactory) Code() string { return "tencent" }
func (f *tencentFactory) New(ctx any, cfg any) (SMSSender, error) {
	c, ok := cfg.(smsConfig)
	if !ok {
		return nil, fmt.Errorf("expected smsConfig, got %T", cfg)
	}
	return &tencentSMS{apiKey: c.APIKey}, nil
}

type emptyCodeFactory struct{}

func (f *emptyCodeFactory) Code() string { return "" }
func (f *emptyCodeFactory) New(ctx any, cfg any) (SMSSender, error) {
	return &aliyunSMS{}, nil
}

type brokenFactory struct{}

func (f *brokenFactory) Code() string { return "broken" }
func (f *brokenFactory) New(ctx any, cfg any) (SMSSender, error) {
	return nil, fmt.Errorf("always fail")
}

// assertPanics asserts that fn panics. Replaces missing testutil.AssertPanics.
func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic but none occurred")
		}
	}()
	fn()
}

func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Errorf("expected error containing %q, got nil", substr)
		return
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────

func TestNewRegistry(t *testing.T) {
	r := NewRegistry[SMSSender]()
	testutil.AssertEqual(t, 0, r.Count())
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry[SMSSender]()
	testutil.AssertNoError(t, r.Register(&aliyunFactory{}))

	f, ok := r.Get("aliyun")
	testutil.AssertEqual(t, true, ok)
	testutil.AssertEqual(t, "aliyun", f.Code())

	_, ok = r.Get("nonexistent")
	testutil.AssertEqual(t, false, ok)
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry[SMSSender]()
	testutil.AssertNoError(t, r.Register(&aliyunFactory{}))
	err := r.Register(&aliyunFactory{})
	assertErrContains(t, err, "already registered")
}

func TestMustRegister(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &aliyunFactory{})
	testutil.AssertEqual(t, 1, r.Count())

	assertPanics(t, func() {
		MustRegister(r, &aliyunFactory{})
	})
}

func TestRegisterNil(t *testing.T) {
	r := NewRegistry[SMSSender]()
	err := r.Register(nil)
	assertErrContains(t, err, "nil factory")
}

func TestRegisterEmptyCode(t *testing.T) {
	r := NewRegistry[SMSSender]()
	err := r.Register(&emptyCodeFactory{})
	assertErrContains(t, err, "Code() must not be empty")
}

func TestList(t *testing.T) {
	r := NewRegistry[SMSSender]()
	testutil.AssertNoError(t, r.Register(&tencentFactory{}))
	testutil.AssertNoError(t, r.Register(&aliyunFactory{}))

	codes := r.List()
	testutil.AssertEqual(t, 2, len(codes))
	testutil.AssertEqual(t, "tencent", codes[0])
	testutil.AssertEqual(t, "aliyun", codes[1])
}

func TestCreate(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &aliyunFactory{})

	sender, err := r.Create(nil, "aliyun", smsConfig{APIKey: "ak-123"})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "aliyun:13800138000:hello", sender.Send("13800138000", "hello"))
}

func TestCreateNotFound(t *testing.T) {
	r := NewRegistry[SMSSender]()
	_, err := r.Create(nil, "nonexistent", nil)
	assertErrContains(t, err, "not found")
}

func TestMustCreate(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &aliyunFactory{})

	sender := r.MustCreate(nil, "aliyun", smsConfig{APIKey: "ak-123"})
	testutil.AssertEqual(t, "aliyun:13800138000:hello", sender.Send("13800138000", "hello"))
}

func TestMustCreatePanics(t *testing.T) {
	r := NewRegistry[SMSSender]()
	assertPanics(t, func() {
		r.MustCreate(nil, "nonexistent", nil)
	})
}

func TestCount(t *testing.T) {
	r := NewRegistry[SMSSender]()
	testutil.AssertEqual(t, 0, r.Count())
	MustRegister(r, &aliyunFactory{})
	testutil.AssertEqual(t, 1, r.Count())
	MustRegister(r, &tencentFactory{})
	testutil.AssertEqual(t, 2, r.Count())
}

func TestForEach(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &tencentFactory{})
	MustRegister(r, &aliyunFactory{})

	var codes []string
	err := r.ForEach(func(code string, f Factory[SMSSender]) error {
		codes = append(codes, code)
		return nil
	})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 2, len(codes))
	testutil.AssertEqual(t, "tencent", codes[0])
	testutil.AssertEqual(t, "aliyun", codes[1])
}

func TestForEachEarlyExit(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &aliyunFactory{})
	MustRegister(r, &tencentFactory{})

	count := 0
	_ = r.ForEach(func(code string, f Factory[SMSSender]) error {
		count++
		return fmt.Errorf("stop at %s", code)
	})
	testutil.AssertEqual(t, 1, count)
}

func TestReset(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &aliyunFactory{})
	testutil.AssertEqual(t, 1, r.Count())
	r.Reset()
	testutil.AssertEqual(t, 0, r.Count())
	_, ok := r.Get("aliyun")
	testutil.AssertEqual(t, false, ok)
}

func TestConcurrentReads(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &aliyunFactory{})
	MustRegister(r, &tencentFactory{})

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.List()
			r.Get("aliyun")
			r.Count()
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			r.List()
			r.Get("tencent")
			r.Count()
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

func TestCreateFactoryError(t *testing.T) {
	r := NewRegistry[SMSSender]()
	MustRegister(r, &brokenFactory{})

	_, err := r.Create(nil, "broken", nil)
	assertErrContains(t, err, "always fail")
}
