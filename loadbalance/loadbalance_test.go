package loadbalance_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/discovery"
	"github.com/astra-go/astra/loadbalance"
	"github.com/astra-go/astra/testutil"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func instances(addrs ...string) []*discovery.ServiceInstance {
	out := make([]*discovery.ServiceInstance, len(addrs))
	for i, a := range addrs {
		out[i] = &discovery.ServiceInstance{ID: a, Address: a}
	}
	return out
}

// ─── RoundRobin ───────────────────────────────────────────────────────────────

func TestRoundRobin_CyclesInstances(t *testing.T) {
	lb := loadbalance.NewRoundRobin()
	insts := instances("a:80", "b:80", "c:80")

	expected := []string{"a:80", "b:80", "c:80", "a:80", "b:80"}
	for i, want := range expected {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		if got.Address != want {
			t.Errorf("pick %d: want %s, got %s", i, want, got.Address)
		}
	}
}

func TestRoundRobin_SingleInstance_AlwaysSame(t *testing.T) {
	lb := loadbalance.NewRoundRobin()
	insts := instances("only:80")

	for i := 0; i < 5; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "only:80", got.Address)
	}
}

func TestRoundRobin_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewRoundRobin()
	_, err := lb.Pick(nil, "")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

func TestRoundRobin_Concurrent(t *testing.T) {
	lb := loadbalance.NewRoundRobin()
	insts := instances("x:80", "y:80", "z:80")

	var wg sync.WaitGroup
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := lb.Pick(insts, "")
			if err != nil || got == nil {
				t.Errorf("concurrent Pick failed: err=%v, got=%v", err, got)
			}
		}()
	}
	wg.Wait()
}

// ─── Random ───────────────────────────────────────────────────────────────────

func TestRandom_ReturnsValidInstance(t *testing.T) {
	lb := loadbalance.NewRandom()
	insts := instances("a:80", "b:80", "c:80")
	addrs := map[string]bool{"a:80": true, "b:80": true, "c:80": true}

	for i := 0; i < 20; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		if !addrs[got.Address] {
			t.Errorf("unexpected address: %s", got.Address)
		}
	}
}

func TestRandom_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewRandom()
	_, err := lb.Pick([]*discovery.ServiceInstance{}, "")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

// ─── Weighted ─────────────────────────────────────────────────────────────────

func TestWeighted_ReturnsValidInstance(t *testing.T) {
	lb := loadbalance.NewWeighted()
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Weight: 10},
		{ID: "b", Address: "b:80", Weight: 1},
	}
	seen := map[string]int{}
	for i := 0; i < 100; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		seen[got.Address]++
	}
	// "a" should appear much more often than "b" (~10× weight).
	if seen["a:80"] <= seen["b:80"] {
		t.Errorf("expected a:80 to dominate; got a=%d, b=%d", seen["a:80"], seen["b:80"])
	}
}

func TestWeighted_SingleInstance_AlwaysChosen(t *testing.T) {
	lb := loadbalance.NewWeighted()
	insts := []*discovery.ServiceInstance{{ID: "only", Address: "only:80", Weight: 5}}
	for i := 0; i < 5; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "only:80", got.Address)
	}
}

func TestWeighted_ZeroWeight_TreatedAsOne(t *testing.T) {
	lb := loadbalance.NewWeighted()
	// Both have weight 0 → treated as weight 1 each.
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Weight: 0},
		{ID: "b", Address: "b:80", Weight: 0},
	}
	seen := map[string]int{}
	for i := 0; i < 100; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		seen[got.Address]++
	}
	// With equal weights, both should be selected roughly equally (not perfectly split).
	if seen["a:80"] == 0 || seen["b:80"] == 0 {
		t.Errorf("expected both instances to be selected; got a=%d, b=%d", seen["a:80"], seen["b:80"])
	}
}

func TestWeighted_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewWeighted()
	_, err := lb.Pick(nil, "")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

// ─── LeastConn ────────────────────────────────────────────────────────────────

func TestLeastConn_PicksLeastLoaded(t *testing.T) {
	lb := loadbalance.NewLeastConn()
	insts := instances("a:80", "b:80", "c:80")

	// Pre-load a:80 with 5 in-flight requests, b:80 with 2.
	for i := 0; i < 5; i++ {
		inst, _ := lb.Pick([]*discovery.ServiceInstance{insts[0]}, "")
		_ = inst // intentionally not calling Done — simulates in-flight
	}
	for i := 0; i < 2; i++ {
		lb.Pick([]*discovery.ServiceInstance{insts[1]}, "") //nolint
	}

	// Next pick over all three should choose c:80 (count = 0).
	got, err := lb.Pick(insts, "")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "c:80", got.Address)
}

func TestLeastConn_Done_DecrementsCount(t *testing.T) {
	lb := loadbalance.NewLeastConn()
	insts := instances("a:80", "b:80")

	// Give a:80 2 in-flight and b:80 3 in-flight.
	a, _ := lb.Pick(insts, "")
	a2, _ := lb.Pick(instances("a:80"), "")
	b, _ := lb.Pick(instances("b:80"), "")
	b2, _ := lb.Pick(instances("b:80"), "")
	b3, _ := lb.Pick(instances("b:80"), "")

	// Complete both a requests → a:80 count falls to 0.
	lb.Done(a)
	lb.Done(a2)

	// b:80 still has 3; a:80 has 0 → next pick must be a:80.
	got, err := lb.Pick(insts, "")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "a:80", got.Address)

	// Cleanup to avoid test leakage.
	lb.Done(b)
	lb.Done(b2)
	lb.Done(b3)
}

func TestLeastConn_Done_NilIsNoop(t *testing.T) {
	lb := loadbalance.NewLeastConn()
	lb.Done(nil) // must not panic
}

func TestLeastConn_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewLeastConn()
	_, err := lb.Pick(nil, "")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

func TestLeastConn_Concurrent(t *testing.T) {
	lb := loadbalance.NewLeastConn()
	insts := instances("x:80", "y:80", "z:80")

	var wg sync.WaitGroup
	for i := 0; i < 300; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst, err := lb.Pick(insts, "")
			if err != nil || inst == nil {
				t.Errorf("concurrent Pick failed: err=%v inst=%v", err, inst)
				return
			}
			lb.Done(inst)
		}()
	}
	wg.Wait()
}

func TestLeastConn_SingleInstance_AlwaysChosen(t *testing.T) {
	lb := loadbalance.NewLeastConn()
	insts := instances("only:80")
	for i := 0; i < 5; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "only:80", got.Address)
		lb.Done(got)
	}
}

// ─── ConsistentHash ───────────────────────────────────────────────────────────

func TestConsistentHash_SameKeyAlwaysSameInstance(t *testing.T) {
	lb := loadbalance.NewConsistentHash(50)
	insts := instances("s1:80", "s2:80", "s3:80")

	const key = "session-abc-123"
	first, err := lb.Pick(insts, key)
	testutil.AssertNoError(t, err)

	for i := 0; i < 20; i++ {
		got, err := lb.Pick(insts, key)
		testutil.AssertNoError(t, err)
		if got.Address != first.Address {
			t.Errorf("inconsistent hash: got %s, expected %s on call %d",
				got.Address, first.Address, i)
		}
	}
}

func TestConsistentHash_DifferentKeysCanMapDifferently(t *testing.T) {
	lb := loadbalance.NewConsistentHash(150)
	insts := instances("s1:80", "s2:80", "s3:80")

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		got, _ := lb.Pick(insts, fmt.Sprintf("key-%d", i))
		seen[got.Address] = true
	}
	// With 100 different keys, we expect at least 2 different instances selected.
	if len(seen) < 2 {
		t.Errorf("consistent hash mapped all keys to same instance: %v", seen)
	}
}

func TestConsistentHash_EmptyKey_FallsBackToRandom(t *testing.T) {
	lb := loadbalance.NewConsistentHash(50)
	insts := instances("a:80", "b:80")
	got, err := lb.Pick(insts, "")
	testutil.AssertNoError(t, err)
	if got == nil {
		t.Error("expected a result when key is empty")
	}
}

func TestConsistentHash_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewConsistentHash(50)
	_, err := lb.Pick(nil, "key")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

func TestConsistentHash_DefaultReplicas(t *testing.T) {
	lb := loadbalance.NewConsistentHash(0) // 0 → default 150
	insts := instances("x:80")
	got, err := lb.Pick(insts, "k")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "x:80", got.Address)
}

func TestConsistentHash_RingCached_SameInstances(t *testing.T) {
	// Verify that calling Pick repeatedly with the same instance list reuses
	// the cached ring (no observable behavioral difference — just must not panic
	// and must remain consistent).
	lb := loadbalance.NewConsistentHash(100)
	insts := instances("n1:80", "n2:80", "n3:80")

	const key = "sticky-user"
	first, _ := lb.Pick(insts, key)
	for i := 0; i < 50; i++ {
		got, err := lb.Pick(insts, key)
		testutil.AssertNoError(t, err)
		if got.Address != first.Address {
			t.Errorf("ring changed unexpectedly on call %d", i)
		}
	}
}

func TestConsistentHash_RingRebuilt_WhenInstancesChange(t *testing.T) {
	// Adding a new instance must trigger a ring rebuild (new mapping is acceptable).
	lb := loadbalance.NewConsistentHash(50)
	set1 := instances("a:80", "b:80")
	set2 := instances("a:80", "b:80", "c:80")

	const key = "user-42"
	// Pick from set1 — remember result.
	lb.Pick(set1, key) //nolint

	// Pick from set2 — must not panic; result may differ.
	got, err := lb.Pick(set2, key)
	testutil.AssertNoError(t, err)
	if got == nil {
		t.Error("expected non-nil result after ring rebuild")
	}
}

// ─── Filter ───────────────────────────────────────────────────────────────────

func TestFilter_KeepsMatchingInstances(t *testing.T) {
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Metadata: map[string]string{"zone": "us-east"}},
		{ID: "b", Address: "b:80", Metadata: map[string]string{"zone": "eu-west"}},
		{ID: "c", Address: "c:80", Metadata: map[string]string{"zone": "us-east"}},
	}
	got := loadbalance.Filter(insts, func(i *discovery.ServiceInstance) bool {
		return i.Metadata["zone"] == "us-east"
	})
	if len(got) != 2 {
		t.Fatalf("Filter: want 2, got %d", len(got))
	}
	for _, inst := range got {
		if inst.Metadata["zone"] != "us-east" {
			t.Errorf("Filter: unexpected instance %s", inst.ID)
		}
	}
}

func TestFilter_EmptyInput_ReturnsEmpty(t *testing.T) {
	got := loadbalance.Filter(nil, func(*discovery.ServiceInstance) bool { return true })
	if len(got) != 0 {
		t.Errorf("Filter(nil): expected empty, got %v", got)
	}
}

func TestFilter_NoneMatch_ReturnsEmpty(t *testing.T) {
	insts := instances("a:80", "b:80")
	got := loadbalance.Filter(insts, func(*discovery.ServiceInstance) bool { return false })
	if len(got) != 0 {
		t.Errorf("Filter(none match): expected empty, got %v", got)
	}
}

func TestFilterByMetadata_ReturnsMatchingInstances(t *testing.T) {
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Metadata: map[string]string{"status": "active"}},
		{ID: "b", Address: "b:80", Metadata: map[string]string{"status": "draining"}},
		{ID: "c", Address: "c:80", Metadata: map[string]string{"status": "active"}},
	}
	got := loadbalance.FilterByMetadata(insts, "status", "active")
	if len(got) != 2 {
		t.Fatalf("FilterByMetadata: want 2, got %d", len(got))
	}
}

func TestFilterByMetadata_IntegratesWithPick(t *testing.T) {
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Metadata: map[string]string{"status": "active"}},
		{ID: "b", Address: "b:80", Metadata: map[string]string{"status": "draining"}},
	}
	healthy := loadbalance.FilterByMetadata(insts, "status", "active")
	lb := loadbalance.NewRoundRobin()
	got, err := lb.Pick(healthy, "")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "a:80", got.Address)
}

// ─── Benchmarks ───────────────────────────────────────────────────────────────

var benchInstances = func() []*discovery.ServiceInstance {
	insts := make([]*discovery.ServiceInstance, 10)
	for i := range insts {
		insts[i] = &discovery.ServiceInstance{
			ID:      fmt.Sprintf("node-%d", i),
			Address: fmt.Sprintf("10.0.0.%d:8080", i),
			Weight:  i + 1,
		}
	}
	return insts
}()

func BenchmarkRoundRobin(b *testing.B) {
	lb := loadbalance.NewRoundRobin()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Pick(benchInstances, "") //nolint
		}
	})
}

func BenchmarkRandom(b *testing.B) {
	lb := loadbalance.NewRandom()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Pick(benchInstances, "") //nolint
		}
	})
}

func BenchmarkWeighted(b *testing.B) {
	lb := loadbalance.NewWeighted()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Pick(benchInstances, "") //nolint
		}
	})
}

func BenchmarkLeastConn(b *testing.B) {
	lb := loadbalance.NewLeastConn()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			inst, _ := lb.Pick(benchInstances, "")
			lb.Done(inst)
		}
	})
}

func BenchmarkConsistentHash_StableRing(b *testing.B) {
	// Stable instance list — ring should be served from cache after first build.
	lb := loadbalance.NewConsistentHash(150)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			lb.Pick(benchInstances, fmt.Sprintf("user-%d", i%1000)) //nolint
			i++
		}
	})
}

func BenchmarkConsistentHash_ChangingRing(b *testing.B) {
	// Alternating two instance sets — forces ring rebuild every other call.
	lb := loadbalance.NewConsistentHash(150)
	set1 := benchInstances[:5]
	set2 := benchInstances[5:]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			lb.Pick(set1, fmt.Sprintf("key-%d", i)) //nolint
		} else {
			lb.Pick(set2, fmt.Sprintf("key-%d", i)) //nolint
		}
	}
}

// ─── SmoothWeighted (SWRR) ────────────────────────────────────────────────────

func instancesWeighted(specs ...struct{ addr string; w int }) []*discovery.ServiceInstance {
	out := make([]*discovery.ServiceInstance, len(specs))
	for i, s := range specs {
		out[i] = &discovery.ServiceInstance{ID: s.addr, Address: s.addr, Weight: s.w}
	}
	return out
}

func TestSmoothWeighted_Distribution(t *testing.T) {
	// weights [5, 3, 2] → 100 picks should approximate 50 / 30 / 20 counts.
	lb := loadbalance.NewSmoothWeighted()
	insts := instancesWeighted(
		struct{ addr string; w int }{"a:80", 5},
		struct{ addr string; w int }{"b:80", 3},
		struct{ addr string; w int }{"c:80", 2},
	)
	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		counts[got.Address]++
	}
	// Tolerant check: each instance count should be within ±15 of the expected.
	want := map[string]int{"a:80": 50, "b:80": 30, "c:80": 20}
	for addr, exp := range want {
		got := counts[addr]
		diff := exp - got
		if diff < 0 {
			diff = -diff
		}
		if diff > 15 {
			t.Errorf("SWRR distribution for %s: want ~%d, got %d", addr, exp, got)
		}
	}
}

func TestSmoothWeighted_Smoothness_NoBurst(t *testing.T) {
	// The smooth variant must not deliver 5 consecutive "a:80" picks for weights [5,3,2].
	lb := loadbalance.NewSmoothWeighted()
	insts := instancesWeighted(
		struct{ addr string; w int }{"a:80", 5},
		struct{ addr string; w int }{"b:80", 3},
		struct{ addr string; w int }{"c:80", 2},
	)
	maxConsecutive := 0
	consecutive := 0
	prev := ""
	for i := 0; i < 100; i++ {
		got, _ := lb.Pick(insts, "")
		if got.Address == prev {
			consecutive++
			if consecutive > maxConsecutive {
				maxConsecutive = consecutive
			}
		} else {
			consecutive = 1
			prev = got.Address
		}
	}
	// SWRR must never deliver more than 2 consecutive picks of the same instance
	// for weights [5,3,2] over 100 requests.
	if maxConsecutive > 2 {
		t.Errorf("SWRR burst: max consecutive picks = %d, want ≤ 2", maxConsecutive)
	}
}

func TestSmoothWeighted_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewSmoothWeighted()
	_, err := lb.Pick(nil, "")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

func TestSmoothWeighted_SingleInstance_AlwaysChosen(t *testing.T) {
	lb := loadbalance.NewSmoothWeighted()
	insts := instancesWeighted(struct{ addr string; w int }{"only:80", 10})
	for i := 0; i < 5; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "only:80", got.Address)
	}
}

func TestSmoothWeighted_Concurrent(t *testing.T) {
	lb := loadbalance.NewSmoothWeighted()
	insts := instancesWeighted(
		struct{ addr string; w int }{"x:80", 2},
		struct{ addr string; w int }{"y:80", 3},
	)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := lb.Pick(insts, "")
			if err != nil || got == nil {
				t.Errorf("concurrent SWRR Pick failed: err=%v", err)
			}
		}()
	}
	wg.Wait()
}

// ─── P2C ─────────────────────────────────────────────────────────────────────

func TestP2C_PicksLowerLoaded(t *testing.T) {
	// Create a P2C with one heavily loaded and one idle instance.
	// P2C should consistently prefer the idle one when it samples those two.
	lb := loadbalance.NewP2C()
	heavy := &discovery.ServiceInstance{ID: "heavy:80", Address: "heavy:80"}
	idle := &discovery.ServiceInstance{ID: "idle:80", Address: "idle:80"}
	insts := []*discovery.ServiceInstance{heavy, idle}

	// Simulate 20 in-flight requests on heavy:80 via RecordSuccess loop.
	for i := 0; i < 20; i++ {
		lb.RecordSuccess(heavy, 500*time.Millisecond) // high latency
	}

	idleCount := 0
	for i := 0; i < 50; i++ {
		got, err := lb.Pick(insts, "")
		testutil.AssertNoError(t, err)
		if got.Address == "idle:80" {
			idleCount++
		}
		lb.RecordSuccess(got, time.Millisecond)
	}
	// With heavy:80 marked as slow, idle:80 should win the majority.
	if idleCount < 30 {
		t.Errorf("P2C: expected idle:80 to win majority, got %d/50", idleCount)
	}
}

func TestP2C_EWMAAdaptsToLatency(t *testing.T) {
	// After recording many low-latency successes on "fast", it should score lower
	// than "slow" (which has never had a success recorded = default 1ms).
	lb := loadbalance.NewP2C()
	fast := &discovery.ServiceInstance{ID: "fast:80", Address: "fast:80"}
	slow := &discovery.ServiceInstance{ID: "slow:80", Address: "slow:80"}
	insts := []*discovery.ServiceInstance{fast, slow}

	// Warm up "slow" with high latency.
	for i := 0; i < 10; i++ {
		lb.RecordSuccess(slow, 200*time.Millisecond)
	}
	// "fast" stays at default ewma (1ms), so should score better.
	fastCount := 0
	for i := 0; i < 40; i++ {
		got, _ := lb.Pick(insts, "")
		if got.Address == "fast:80" {
			fastCount++
		}
		lb.RecordSuccess(got, time.Millisecond)
	}
	if fastCount < 20 {
		t.Errorf("P2C EWMA: expected fast:80 to dominate, got %d/40", fastCount)
	}
}

func TestP2C_Reporter_Interface(t *testing.T) {
	lb := loadbalance.NewP2C()
	// P2C must implement Reporter.
	var _ loadbalance.Reporter = lb
	inst := &discovery.ServiceInstance{ID: "x:80", Address: "x:80"}
	// Both should not panic and not error.
	lb.RecordSuccess(inst, 10*time.Millisecond)
	lb.RecordError(inst, 50*time.Millisecond)
	lb.RecordSuccess(nil, time.Millisecond) // nil must be a no-op
	lb.RecordError(nil, time.Millisecond)
}

func TestP2C_Done_DecrementsInflight(t *testing.T) {
	lb := loadbalance.NewP2C()
	insts := []*discovery.ServiceInstance{
		{ID: "a:80", Address: "a:80"},
		{ID: "b:80", Address: "b:80"},
	}
	for i := 0; i < 5; i++ {
		inst, _ := lb.Pick(insts, "")
		lb.Done(inst)
	}
	// After done, there should be no accumulated inflight — just verify no panic.
}

func TestP2C_SingleInstance(t *testing.T) {
	lb := loadbalance.NewP2C()
	inst := &discovery.ServiceInstance{ID: "only:80", Address: "only:80"}
	got, err := lb.Pick([]*discovery.ServiceInstance{inst}, "")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "only:80", got.Address)
	lb.Done(got)
}

func TestP2C_EmptyList_ReturnsError(t *testing.T) {
	lb := loadbalance.NewP2C()
	_, err := lb.Pick(nil, "")
	testutil.AssertErrorIs(t, err, loadbalance.ErrNoInstances)
}

// ─── LocalityFirst ────────────────────────────────────────────────────────────

func TestLocalityFirst_PrefersLocal(t *testing.T) {
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Metadata: map[string]string{"zone": "us-east-1"}},
		{ID: "b", Address: "b:80", Metadata: map[string]string{"zone": "eu-west-1"}},
		{ID: "c", Address: "c:80", Metadata: map[string]string{"zone": "us-east-1"}},
	}
	got := loadbalance.LocalityFirst(insts, "zone", "us-east-1")
	if len(got) != 2 {
		t.Fatalf("LocalityFirst: want 2 local instances, got %d", len(got))
	}
	for _, inst := range got {
		if inst.Metadata["zone"] != "us-east-1" {
			t.Errorf("LocalityFirst: unexpected non-local instance %s", inst.ID)
		}
	}
}

func TestLocalityFirst_FallsBackToAll_WhenNoLocal(t *testing.T) {
	insts := []*discovery.ServiceInstance{
		{ID: "a", Address: "a:80", Metadata: map[string]string{"zone": "eu-west-1"}},
		{ID: "b", Address: "b:80", Metadata: map[string]string{"zone": "ap-east-1"}},
	}
	// No "us-east-1" instances — must return full list.
	got := loadbalance.LocalityFirst(insts, "zone", "us-east-1")
	if len(got) != len(insts) {
		t.Errorf("LocalityFirst fallback: want %d, got %d", len(insts), len(got))
	}
}

func TestLocalityFirst_EmptyInput_ReturnsEmpty(t *testing.T) {
	got := loadbalance.LocalityFirst(nil, "zone", "us-east-1")
	if len(got) != 0 {
		t.Errorf("LocalityFirst(nil): expected empty, got %v", got)
	}
}

// ─── Benchmarks (new strategies) ─────────────────────────────────────────────

func BenchmarkSmoothWeighted(b *testing.B) {
	lb := loadbalance.NewSmoothWeighted()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Pick(benchInstances, "") //nolint
		}
	})
}

func BenchmarkP2C(b *testing.B) {
	lb := loadbalance.NewP2C()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			inst, _ := lb.Pick(benchInstances, "")
			lb.Done(inst)
		}
	})
}
