package lo_test

import (
	"testing"

	"github.com/astra-go/astra/lo"
)

// ─── Min / Max ────────────────────────────────────────────────────────────────

func TestMin(t *testing.T) {
	if lo.Min([]int{3, 1, 4, 1, 5}) != 1 {
		t.Error("Min: want 1")
	}
	if lo.Min([]string{"banana", "apple", "cherry"}) != "apple" {
		t.Error("Min(strings)")
	}
}

func TestMinPanicOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Min(empty): expected panic")
		}
	}()
	lo.Min([]int{})
}

func TestMinBy(t *testing.T) {
	type P struct{ Name string; Age int }
	people := []P{{"alice", 30}, {"bob", 25}, {"carol", 35}}
	youngest := lo.MinBy(people, func(a, b P) bool { return a.Age < b.Age })
	if youngest.Name != "bob" {
		t.Errorf("MinBy: want bob, got %s", youngest.Name)
	}
}

func TestMax(t *testing.T) {
	if lo.Max([]float64{1.5, 3.7, 2.1}) != 3.7 {
		t.Error("Max: want 3.7")
	}
}

func TestMaxPanicOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Max(empty): expected panic")
		}
	}()
	lo.Max([]int{})
}

func TestMaxBy(t *testing.T) {
	words := []string{"hi", "hello", "hey"}
	longest := lo.MaxBy(words, func(a, b string) bool { return len(a) > len(b) })
	if longest != "hello" {
		t.Errorf("MaxBy: want hello, got %s", longest)
	}
}

// ─── Sum / SumBy ─────────────────────────────────────────────────────────────

func TestSum(t *testing.T) {
	if lo.Sum([]int{1, 2, 3, 4}) != 10 {
		t.Error("Sum: want 10")
	}
}

func TestSum_Empty(t *testing.T) {
	if lo.Sum([]int{}) != 0 {
		t.Error("Sum(empty): want 0")
	}
}

func TestSumBy(t *testing.T) {
	type Item struct{ Price float64 }
	items := []Item{{1.5}, {2.0}, {0.5}}
	total := lo.SumBy(items, func(i Item) float64 { return i.Price })
	if total != 4.0 {
		t.Errorf("SumBy: want 4.0, got %f", total)
	}
}

// ─── Clamp ───────────────────────────────────────────────────────────────────

func TestClamp_BelowMin(t *testing.T) {
	if lo.Clamp(-5, 0, 100) != 0 {
		t.Error("Clamp below min: want 0")
	}
}

func TestClamp_AboveMax(t *testing.T) {
	if lo.Clamp(150, 0, 100) != 100 {
		t.Error("Clamp above max: want 100")
	}
}

func TestClamp_InRange(t *testing.T) {
	if lo.Clamp(42, 0, 100) != 42 {
		t.Error("Clamp in range: want 42")
	}
}

func TestClamp_Strings(t *testing.T) {
	if lo.Clamp("m", "a", "z") != "m" {
		t.Error("Clamp(strings): want m")
	}
}
