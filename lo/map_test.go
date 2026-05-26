package lo_test

import (
	"fmt"
	"testing"

	"github.com/astra-go/astra/lo"
)

// ─── Keys / Values ────────────────────────────────────────────────────────────

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	keys := lo.Keys(m)
	if len(keys) != 3 {
		t.Errorf("Keys: want 3 keys, got %d", len(keys))
	}
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 10, "b": 20}
	vals := lo.Values(m)
	if len(vals) != 2 {
		t.Errorf("Values: want 2 values, got %d", len(vals))
	}
}

// ─── Entries / FromEntries ────────────────────────────────────────────────────

func TestEntries_FromEntries_RoundTrip(t *testing.T) {
	original := map[string]int{"x": 1, "y": 2}
	entries := lo.Entries(original)
	if len(entries) != 2 {
		t.Fatalf("Entries: want 2, got %d", len(entries))
	}
	rebuilt := lo.FromEntries(entries)
	for k, v := range original {
		if rebuilt[k] != v {
			t.Errorf("FromEntries: key %q: want %d, got %d", k, v, rebuilt[k])
		}
	}
}

// ─── MapKeys / MapValues ──────────────────────────────────────────────────────

func TestMapKeys(t *testing.T) {
	m := map[int]string{1: "a", 2: "b"}
	got := lo.MapKeys(m, func(_ string, k int) string { return fmt.Sprintf("k%d", k) })
	if got["k1"] != "a" || got["k2"] != "b" {
		t.Errorf("MapKeys: got %v", got)
	}
}

func TestMapValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	got := lo.MapValues(m, func(v int, _ string) int { return v * 2 })
	if got["a"] != 2 || got["b"] != 4 {
		t.Errorf("MapValues: got %v", got)
	}
}

// ─── PickBy / OmitBy ─────────────────────────────────────────────────────────

func TestPickBy(t *testing.T) {
	m := map[string]int{"a": 1, "b": -1, "c": 2}
	got := lo.PickBy(m, func(v int, _ string) bool { return v > 0 })
	if len(got) != 2 || got["b"] != 0 {
		t.Errorf("PickBy: got %v", got)
	}
}

func TestOmitBy(t *testing.T) {
	m := map[string]int{"a": 1, "b": -1, "c": 2}
	got := lo.OmitBy(m, func(v int, _ string) bool { return v < 0 })
	if len(got) != 2 || lo.Has(got, "b") {
		t.Errorf("OmitBy: got %v", got)
	}
}

// ─── Invert / Assign ─────────────────────────────────────────────────────────

func TestInvert(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	inv := lo.Invert(m)
	if inv[1] != "a" || inv[2] != "b" {
		t.Errorf("Invert: got %v", inv)
	}
}

func TestAssign(t *testing.T) {
	m1 := map[string]int{"a": 1}
	m2 := map[string]int{"b": 2}
	m3 := map[string]int{"a": 99, "c": 3} // overrides m1["a"]
	got := lo.Assign(m1, m2, m3)
	if got["a"] != 99 || got["b"] != 2 || got["c"] != 3 {
		t.Errorf("Assign: got %v", got)
	}
}

// ─── Has / MapToSlice ─────────────────────────────────────────────────────────

func TestHas(t *testing.T) {
	m := map[string]int{"key": 42}
	if !lo.Has(m, "key") {
		t.Error("Has: expected true for existing key")
	}
	if lo.Has(m, "missing") {
		t.Error("Has: expected false for missing key")
	}
}

func TestMapToSlice(t *testing.T) {
	m := map[string]int{"a": 1}
	got := lo.MapToSlice(m, func(k string, v int) string { return fmt.Sprintf("%s=%d", k, v) })
	if len(got) != 1 || got[0] != "a=1" {
		t.Errorf("MapToSlice: got %v", got)
	}
}
