package lo_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/astra-go/astra/lo"
)

// ─── Map ──────────────────────────────────────────────────────────────────────

func TestMap_IntToString(t *testing.T) {
	got := lo.Map([]int{1, 2, 3}, func(x, _ int) string { return strconv.Itoa(x) })
	want := []string{"1", "2", "3"}
	if !equalSlice(got, want) {
		t.Errorf("Map: want %v, got %v", want, got)
	}
}

func TestMap_EmptySlice(t *testing.T) {
	got := lo.Map([]int{}, func(x, _ int) int { return x })
	if len(got) != 0 {
		t.Errorf("Map(empty): expected empty, got %v", got)
	}
}

// ─── Filter ───────────────────────────────────────────────────────────────────

func TestFilter_Evens(t *testing.T) {
	got := lo.Filter([]int{1, 2, 3, 4, 5}, func(x, _ int) bool { return x%2 == 0 })
	want := []int{2, 4}
	if !equalSlice(got, want) {
		t.Errorf("Filter: want %v, got %v", want, got)
	}
}

func TestFilter_AllRemoved(t *testing.T) {
	got := lo.Filter([]int{1, 3, 5}, func(x, _ int) bool { return x%2 == 0 })
	if len(got) != 0 {
		t.Errorf("Filter all removed: got %v", got)
	}
}

// ─── Reduce ───────────────────────────────────────────────────────────────────

func TestReduce_Sum(t *testing.T) {
	got := lo.Reduce([]int{1, 2, 3, 4}, func(acc, x, _ int) int { return acc + x }, 0)
	if got != 10 {
		t.Errorf("Reduce sum: want 10, got %d", got)
	}
}

func TestReduceRight_Concat(t *testing.T) {
	got := lo.ReduceRight([]string{"a", "b", "c"}, func(acc, s string, _ int) string { return acc + s }, "")
	if got != "cba" {
		t.Errorf("ReduceRight: want 'cba', got %q", got)
	}
}

// ─── ForEach / Times ──────────────────────────────────────────────────────────

func TestForEach_CollectIndices(t *testing.T) {
	var indices []int
	lo.ForEach([]string{"a", "b", "c"}, func(_ string, i int) { indices = append(indices, i) })
	if !equalSlice(indices, []int{0, 1, 2}) {
		t.Errorf("ForEach indices: want [0,1,2], got %v", indices)
	}
}

func TestTimes_Squares(t *testing.T) {
	got := lo.Times(4, func(i int) int { return i * i })
	want := []int{0, 1, 4, 9}
	if !equalSlice(got, want) {
		t.Errorf("Times: want %v, got %v", want, got)
	}
}

// ─── Contains / ContainsBy ────────────────────────────────────────────────────

func TestContains_Found(t *testing.T) {
	if !lo.Contains([]string{"a", "b", "c"}, "b") {
		t.Error("Contains: expected true")
	}
}

func TestContains_NotFound(t *testing.T) {
	if lo.Contains([]int{1, 2, 3}, 99) {
		t.Error("Contains: expected false")
	}
}

func TestContainsBy(t *testing.T) {
	if !lo.ContainsBy([]int{1, 2, 3}, func(x int) bool { return x > 2 }) {
		t.Error("ContainsBy: expected true")
	}
}

// ─── Every / Some / None ──────────────────────────────────────────────────────

func TestEvery_AllPositive(t *testing.T) {
	if !lo.Every([]int{1, 2, 3}, func(x int) bool { return x > 0 }) {
		t.Error("Every: expected true")
	}
}

func TestEvery_OneFails(t *testing.T) {
	if lo.Every([]int{1, -1, 3}, func(x int) bool { return x > 0 }) {
		t.Error("Every: expected false")
	}
}

func TestEvery_EmptyIsTrue(t *testing.T) {
	if !lo.Every([]int{}, func(x int) bool { return x > 0 }) {
		t.Error("Every(empty): expected vacuously true")
	}
}

func TestSome(t *testing.T) {
	if !lo.Some([]int{-1, 0, 3}, func(x int) bool { return x > 0 }) {
		t.Error("Some: expected true")
	}
}

func TestNone(t *testing.T) {
	if !lo.None([]int{1, 2, 3}, func(x int) bool { return x < 0 }) {
		t.Error("None: expected true")
	}
}

// ─── Count / CountBy ─────────────────────────────────────────────────────────

func TestCount(t *testing.T) {
	n := lo.Count([]int{1, 2, 1, 3, 1}, 1)
	if n != 3 {
		t.Errorf("Count: want 3, got %d", n)
	}
}

func TestCountBy(t *testing.T) {
	n := lo.CountBy([]int{1, 2, 3, 4, 5}, func(x int) bool { return x%2 == 0 })
	if n != 2 {
		t.Errorf("CountBy: want 2, got %d", n)
	}
}

// ─── Find / FindIndexOf / IndexOf ────────────────────────────────────────────

func TestFind_Found(t *testing.T) {
	v, ok := lo.Find([]int{1, 2, 3}, func(x int) bool { return x > 1 })
	if !ok || v != 2 {
		t.Errorf("Find: want (2,true), got (%d,%v)", v, ok)
	}
}

func TestFind_NotFound(t *testing.T) {
	_, ok := lo.Find([]int{1, 2, 3}, func(x int) bool { return x > 10 })
	if ok {
		t.Error("Find: expected not found")
	}
}

func TestFindIndexOf(t *testing.T) {
	v, idx, ok := lo.FindIndexOf([]string{"a", "b", "c"}, func(s string) bool { return s == "b" })
	if !ok || v != "b" || idx != 1 {
		t.Errorf("FindIndexOf: want (b,1,true), got (%s,%d,%v)", v, idx, ok)
	}
}

func TestIndexOf(t *testing.T) {
	if lo.IndexOf([]string{"x", "y", "z"}, "y") != 1 {
		t.Error("IndexOf: expected 1")
	}
	if lo.IndexOf([]string{"x", "y", "z"}, "w") != -1 {
		t.Error("IndexOf(missing): expected -1")
	}
}

func TestLastIndexOf(t *testing.T) {
	if lo.LastIndexOf([]int{1, 2, 3, 2}, 2) != 3 {
		t.Error("LastIndexOf: expected 3")
	}
}

// ─── First / Last ─────────────────────────────────────────────────────────────

func TestFirst(t *testing.T) {
	v, ok := lo.First([]int{10, 20})
	if !ok || v != 10 {
		t.Errorf("First: want (10,true), got (%d,%v)", v, ok)
	}
	_, ok2 := lo.First([]int{})
	if ok2 {
		t.Error("First(empty): expected false")
	}
}

func TestFirstOrDefault(t *testing.T) {
	if lo.FirstOrDefault([]int{}, 99) != 99 {
		t.Error("FirstOrDefault(empty): expected 99")
	}
	if lo.FirstOrDefault([]int{1, 2}, 99) != 1 {
		t.Error("FirstOrDefault: expected 1")
	}
}

func TestLast(t *testing.T) {
	v, ok := lo.Last([]int{10, 20, 30})
	if !ok || v != 30 {
		t.Errorf("Last: want (30,true), got (%d,%v)", v, ok)
	}
}

func TestLastOrDefault(t *testing.T) {
	if lo.LastOrDefault([]int{}, 7) != 7 {
		t.Error("LastOrDefault(empty): expected 7")
	}
}

// ─── Take / Drop ─────────────────────────────────────────────────────────────

func TestTake(t *testing.T) {
	if !equalSlice(lo.Take([]int{1, 2, 3, 4}, 2), []int{1, 2}) {
		t.Error("Take(2)")
	}
	if !equalSlice(lo.Take([]int{1, 2}, 10), []int{1, 2}) {
		t.Error("Take(exceed)")
	}
}

func TestTakeRight(t *testing.T) {
	if !equalSlice(lo.TakeRight([]int{1, 2, 3, 4}, 2), []int{3, 4}) {
		t.Error("TakeRight(2)")
	}
}

func TestDrop(t *testing.T) {
	if !equalSlice(lo.Drop([]int{1, 2, 3, 4}, 2), []int{3, 4}) {
		t.Error("Drop(2)")
	}
	if !equalSlice(lo.Drop([]int{1, 2}, 10), []int{}) {
		t.Error("Drop(exceed)")
	}
}

func TestDropRight(t *testing.T) {
	if !equalSlice(lo.DropRight([]int{1, 2, 3, 4}, 2), []int{1, 2}) {
		t.Error("DropRight(2)")
	}
}

// ─── GroupBy / KeyBy / Associate / Partition ─────────────────────────────────

func TestGroupBy(t *testing.T) {
	groups := lo.GroupBy([]int{1, 2, 3, 4, 5}, func(x int) string {
		if x%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if len(groups["even"]) != 2 || len(groups["odd"]) != 3 {
		t.Errorf("GroupBy: got %v", groups)
	}
}

func TestKeyBy(t *testing.T) {
	type P struct{ ID int; Name string }
	people := []P{{1, "alice"}, {2, "bob"}}
	m := lo.KeyBy(people, func(p P) int { return p.ID })
	if m[1].Name != "alice" || m[2].Name != "bob" {
		t.Errorf("KeyBy: got %v", m)
	}
}

func TestAssociate(t *testing.T) {
	m := lo.Associate([]string{"a", "bb", "ccc"}, func(s string) (string, int) { return s, len(s) })
	if m["a"] != 1 || m["bb"] != 2 || m["ccc"] != 3 {
		t.Errorf("Associate: got %v", m)
	}
}

func TestPartition(t *testing.T) {
	big, small := lo.Partition([]int{1, 5, 3, 8, 2}, func(x int) bool { return x > 4 })
	if !equalSlice(big, []int{5, 8}) || !equalSlice(small, []int{1, 3, 2}) {
		t.Errorf("Partition: big=%v small=%v", big, small)
	}
}

// ─── Uniq / UniqBy ───────────────────────────────────────────────────────────

func TestUniq(t *testing.T) {
	got := lo.Uniq([]int{1, 2, 1, 3, 2})
	if !equalSlice(got, []int{1, 2, 3}) {
		t.Errorf("Uniq: want [1,2,3], got %v", got)
	}
}

func TestUniqBy(t *testing.T) {
	got := lo.UniqBy([]string{"Hello", "WORLD", "hello"}, strings.ToLower)
	if !equalSlice(got, []string{"Hello", "WORLD"}) {
		t.Errorf("UniqBy: want [Hello WORLD], got %v", got)
	}
}

// ─── Set operations ───────────────────────────────────────────────────────────

func TestIntersect(t *testing.T) {
	got := lo.Intersect([]int{1, 2, 3}, []int{2, 3, 4})
	if !equalSlice(got, []int{2, 3}) {
		t.Errorf("Intersect: want [2,3], got %v", got)
	}
}

func TestDifference(t *testing.T) {
	only1, only2 := lo.Difference([]int{1, 2, 3}, []int{2, 3, 4})
	if !equalSlice(only1, []int{1}) || !equalSlice(only2, []int{4}) {
		t.Errorf("Difference: only1=%v only2=%v", only1, only2)
	}
}

func TestUnion(t *testing.T) {
	got := lo.Union([]int{1, 2}, []int{2, 3}, []int{3, 4})
	if !equalSlice(got, []int{1, 2, 3, 4}) {
		t.Errorf("Union: want [1,2,3,4], got %v", got)
	}
}

func TestWithout(t *testing.T) {
	got := lo.Without([]int{1, 2, 3, 4}, 2, 4)
	if !equalSlice(got, []int{1, 3}) {
		t.Errorf("Without: want [1,3], got %v", got)
	}
}

// ─── Shape ───────────────────────────────────────────────────────────────────

func TestFlatten(t *testing.T) {
	got := lo.Flatten([][]int{{1, 2}, {3}, {4, 5}})
	if !equalSlice(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("Flatten: want [1..5], got %v", got)
	}
}

func TestFlatMap(t *testing.T) {
	got := lo.FlatMap([]int{1, 2, 3}, func(x, _ int) []int { return []int{x, x * 10} })
	want := []int{1, 10, 2, 20, 3, 30}
	if !equalSlice(got, want) {
		t.Errorf("FlatMap: want %v, got %v", want, got)
	}
}

func TestChunk(t *testing.T) {
	got := lo.Chunk([]int{1, 2, 3, 4, 5}, 2)
	if len(got) != 3 || !equalSlice(got[0], []int{1, 2}) || !equalSlice(got[2], []int{5}) {
		t.Errorf("Chunk: got %v", got)
	}
}

func TestReverse(t *testing.T) {
	got := lo.Reverse([]int{1, 2, 3})
	if !equalSlice(got, []int{3, 2, 1}) {
		t.Errorf("Reverse: want [3,2,1], got %v", got)
	}
}

func TestCompact(t *testing.T) {
	got := lo.Compact([]string{"a", "", "b", ""})
	if !equalSlice(got, []string{"a", "b"}) {
		t.Errorf("Compact: want [a b], got %v", got)
	}
}

func TestRepeat(t *testing.T) {
	if !equalSlice(lo.Repeat(3, "x"), []string{"x", "x", "x"}) {
		t.Error("Repeat: unexpected result")
	}
}

func TestFill(t *testing.T) {
	s := make([]int, 4)
	lo.Fill(s, 7)
	if !equalSlice(s, []int{7, 7, 7, 7}) {
		t.Errorf("Fill: got %v", s)
	}
}

func TestShuffle_LengthPreserved(t *testing.T) {
	src := []int{1, 2, 3, 4, 5}
	got := lo.Shuffle(src)
	if len(got) != len(src) {
		t.Errorf("Shuffle: length mismatch: want %d, got %d", len(src), len(got))
	}
	// Original unchanged
	if !equalSlice(src, []int{1, 2, 3, 4, 5}) {
		t.Errorf("Shuffle: modified original slice")
	}
}

// ─── Zip / Unzip ─────────────────────────────────────────────────────────────

func TestZip_Unzip(t *testing.T) {
	pairs := lo.Zip([]string{"a", "b", "c"}, []int{1, 2, 3})
	if len(pairs) != 3 || pairs[1].A != "b" || pairs[1].B != 2 {
		t.Errorf("Zip: got %v", pairs)
	}
	strs, nums := lo.Unzip(pairs)
	if !equalSlice(strs, []string{"a", "b", "c"}) || !equalSlice(nums, []int{1, 2, 3}) {
		t.Errorf("Unzip: strs=%v nums=%v", strs, nums)
	}
}

func TestZip_UnequalLengths(t *testing.T) {
	pairs := lo.Zip([]int{1, 2, 3}, []int{10, 20})
	if len(pairs) != 2 {
		t.Errorf("Zip(unequal): want 2 pairs, got %d", len(pairs))
	}
}

// ─── helper ───────────────────────────────────────────────────────────────────

func equalSlice[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
