package lo_test

import (
	"testing"

	"github.com/astra-go/astra/lo"
)

// ─── ToPtr / FromPtr ─────────────────────────────────────────────────────────

func TestToPtr(t *testing.T) {
	p := lo.ToPtr(42)
	if p == nil || *p != 42 {
		t.Errorf("ToPtr: want 42, got %v", p)
	}
}

func TestToPtr_String(t *testing.T) {
	p := lo.ToPtr("hello")
	if p == nil || *p != "hello" {
		t.Error("ToPtr(string)")
	}
}

func TestFromPtr_NonNil(t *testing.T) {
	n := 7
	if lo.FromPtr(&n) != 7 {
		t.Error("FromPtr: expected 7")
	}
}

func TestFromPtr_Nil(t *testing.T) {
	if lo.FromPtr((*int)(nil)) != 0 {
		t.Error("FromPtr(nil): expected zero value")
	}
}

// ─── FromPtrOr ───────────────────────────────────────────────────────────────

func TestFromPtrOr_Nil(t *testing.T) {
	if lo.FromPtrOr((*string)(nil), "default") != "default" {
		t.Error("FromPtrOr(nil): expected default")
	}
}

func TestFromPtrOr_NonNil(t *testing.T) {
	s := "value"
	if lo.FromPtrOr(&s, "default") != "value" {
		t.Error("FromPtrOr(non-nil): expected value")
	}
}

// ─── EmptyableToPtr ───────────────────────────────────────────────────────────

func TestEmptyableToPtr_Zero(t *testing.T) {
	if lo.EmptyableToPtr("") != nil {
		t.Error("EmptyableToPtr(empty string): expected nil")
	}
	if lo.EmptyableToPtr(0) != nil {
		t.Error("EmptyableToPtr(0): expected nil")
	}
}

func TestEmptyableToPtr_NonZero(t *testing.T) {
	p := lo.EmptyableToPtr("hello")
	if p == nil || *p != "hello" {
		t.Errorf("EmptyableToPtr(non-zero): got %v", p)
	}
}

// ─── ToSlicePtr / FromSlicePtr ───────────────────────────────────────────────

func TestToSlicePtr(t *testing.T) {
	ptrs := lo.ToSlicePtr([]int{1, 2, 3})
	if len(ptrs) != 3 || *ptrs[0] != 1 || *ptrs[2] != 3 {
		t.Errorf("ToSlicePtr: got %v", ptrs)
	}
}

func TestFromSlicePtr(t *testing.T) {
	a, b := 10, 20
	vals := lo.FromSlicePtr([]*int{&a, nil, &b})
	want := []int{10, 0, 20}
	if !equalSlice(vals, want) {
		t.Errorf("FromSlicePtr: want %v, got %v", want, vals)
	}
}

func TestToSlicePtr_IndependentCopies(t *testing.T) {
	src := []int{1, 2, 3}
	ptrs := lo.ToSlicePtr(src)
	src[0] = 99 // mutate original
	if *ptrs[0] == 99 {
		t.Error("ToSlicePtr: pointers should point to copies, not original slice elements")
	}
}
