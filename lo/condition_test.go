package lo_test

import (
	"errors"
	"testing"

	"github.com/astra-go/astra/lo"
)

// ─── Ternary / TernaryF ───────────────────────────────────────────────────────

func TestTernary_True(t *testing.T) {
	if lo.Ternary(true, "yes", "no") != "yes" {
		t.Error("Ternary(true): expected yes")
	}
}

func TestTernary_False(t *testing.T) {
	if lo.Ternary(false, "yes", "no") != "no" {
		t.Error("Ternary(false): expected no")
	}
}

func TestTernaryF_LazyEval(t *testing.T) {
	called := false
	lo.TernaryF(true,
		func() string { return "ok" },
		func() string { called = true; return "fail" },
	)
	if called {
		t.Error("TernaryF: else branch should not be called when condition is true")
	}
}

// ─── If chain ─────────────────────────────────────────────────────────────────

func TestIfChain_FirstBranchMatches(t *testing.T) {
	got := lo.If(true, "A").ElseIf(true, "B").Else("C")
	if got != "A" {
		t.Errorf("If chain: want A, got %s", got)
	}
}

func TestIfChain_SecondBranchMatches(t *testing.T) {
	got := lo.If(false, "A").ElseIf(true, "B").Else("C")
	if got != "B" {
		t.Errorf("If chain: want B, got %s", got)
	}
}

func TestIfChain_FallsToElse(t *testing.T) {
	got := lo.If(false, "A").ElseIf(false, "B").Else("C")
	if got != "C" {
		t.Errorf("If chain: want C, got %s", got)
	}
}

func TestIfF_LazyEval(t *testing.T) {
	calls := 0
	got := lo.IfF(true, func() int { calls++; return 42 }).
		ElseIfF(true, func() int { calls++; return 99 }).
		Else(0)
	if got != 42 || calls != 1 {
		t.Errorf("IfF: want 42 with 1 call, got %d with %d calls", got, calls)
	}
}

func testGrade(score int) string {
	return lo.If(score >= 90, "A").
		ElseIf(score >= 80, "B").
		ElseIf(score >= 70, "C").
		Else("F")
}

func TestIfChain_GradeLogic(t *testing.T) {
	cases := []struct{ score int; want string }{
		{95, "A"}, {85, "B"}, {72, "C"}, {60, "F"},
	}
	for _, c := range cases {
		if got := testGrade(c.score); got != c.want {
			t.Errorf("grade(%d): want %s, got %s", c.score, c.want, got)
		}
	}
}

// ─── Must helpers ─────────────────────────────────────────────────────────────

func TestMust_NoError(t *testing.T) {
	v := lo.Must(42, nil)
	if v != 42 {
		t.Errorf("Must(no error): want 42, got %d", v)
	}
}

func TestMust_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must: expected panic on error")
		}
	}()
	lo.Must(0, errors.New("boom"))
}

func TestMust0_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must0: expected panic")
		}
	}()
	lo.Must0(errors.New("fail"))
}

func TestMust0_NoError(t *testing.T) {
	lo.Must0(nil) // must not panic
}

func TestMust2_Values(t *testing.T) {
	a, b := lo.Must2("hello", 42, nil)
	if a != "hello" || b != 42 {
		t.Errorf("Must2: want (hello,42), got (%s,%d)", a, b)
	}
}

func TestMust2_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must2: expected panic")
		}
	}()
	lo.Must2("", 0, errors.New("fail"))
}

// ─── Try helpers ──────────────────────────────────────────────────────────────

func TestTry_Success(t *testing.T) {
	if !lo.Try(func() error { return nil }) {
		t.Error("Try(success): expected true")
	}
}

func TestTry_Error(t *testing.T) {
	if lo.Try(func() error { return errors.New("oops") }) {
		t.Error("Try(error): expected false")
	}
}

func TestTry_Panic(t *testing.T) {
	if lo.Try(func() error { panic("boom") }) {
		t.Error("Try(panic): expected false")
	}
}

func TestTryCatch_ErrorCaught(t *testing.T) {
	var caught any
	lo.TryCatch(func() error { return errors.New("err") }, func(e any) { caught = e })
	if caught == nil {
		t.Error("TryCatch: expected catch to be called")
	}
}

func TestTryCatch_PanicCaught(t *testing.T) {
	var caught any
	lo.TryCatch(func() error { panic("panic!") }, func(e any) { caught = e })
	if caught == nil {
		t.Error("TryCatch(panic): expected catch to be called")
	}
}

func TestTryWithErrorValue_Success(t *testing.T) {
	result, err, ok := lo.TryWithErrorValue(func() (int, error) { return 7, nil })
	if !ok || result != 7 || err != nil {
		t.Errorf("TryWithErrorValue(success): got (%d,%v,%v)", result, err, ok)
	}
}

func TestTryWithErrorValue_Error(t *testing.T) {
	_, err, ok := lo.TryWithErrorValue(func() (int, error) { return 0, errors.New("fail") })
	if ok || err == nil {
		t.Errorf("TryWithErrorValue(error): expected !ok and err non-nil")
	}
}

func TestTryWithErrorValue_Panic(t *testing.T) {
	_, err, ok := lo.TryWithErrorValue(func() (int, error) { panic("panic value") })
	if ok || err == nil {
		t.Errorf("TryWithErrorValue(panic): expected !ok and err non-nil")
	}
}
