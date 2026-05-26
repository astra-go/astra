package loadbalance_test

import (
	"context"
	"testing"
	"time"

	"github.com/astra-go/astra/discovery"
	"github.com/astra-go/astra/loadbalance"
	"github.com/astra-go/astra/testutil"
)

// ─── Resolver ─────────────────────────────────────────────────────────────────

func TestResolver_ReturnsInitialSnapshot(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	inst := &discovery.ServiceInstance{ID: "svc-1", Name: "svc", Address: "10.0.0.1:8080"}
	_ = reg.Register(ctx, inst)

	r, err := loadbalance.NewResolver(ctx, reg, "svc")
	testutil.AssertNoError(t, err)
	defer r.Close()

	got := r.Instances()
	if len(got) != 1 {
		t.Fatalf("want 1 instance, got %d", len(got))
	}
	testutil.AssertEqual(t, "svc-1", got[0].ID)
}

func TestResolver_UpdatesOnChange(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	inst1 := &discovery.ServiceInstance{ID: "svc-1", Name: "svc", Address: "10.0.0.1:8080"}
	_ = reg.Register(ctx, inst1)

	r, err := loadbalance.NewResolver(ctx, reg, "svc")
	testutil.AssertNoError(t, err)
	defer r.Close()

	// Register a second instance — the resolver must pick up the change.
	inst2 := &discovery.ServiceInstance{ID: "svc-2", Name: "svc", Address: "10.0.0.2:8080"}
	_ = reg.Register(ctx, inst2)

	// Wait for async update (the channel-based Watch delivers promptly).
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(r.Instances()) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	got := r.Instances()
	if len(got) != 2 {
		t.Fatalf("after second register: want 2 instances, got %d", len(got))
	}
}

func TestResolver_Close_StopsUpdates(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	inst := &discovery.ServiceInstance{ID: "svc-1", Name: "svc", Address: "10.0.0.1:8080"}
	_ = reg.Register(ctx, inst)

	r, err := loadbalance.NewResolver(ctx, reg, "svc")
	testutil.AssertNoError(t, err)

	// Close must not block.
	done := make(chan struct{})
	go func() {
		r.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resolver.Close() blocked for > 1s")
	}
}

func TestResolver_Close_SafeToCallTwice(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	_ = reg.Register(ctx, &discovery.ServiceInstance{ID: "x", Name: "svc", Address: "localhost:1"})
	r, err := loadbalance.NewResolver(ctx, reg, "svc")
	testutil.AssertNoError(t, err)

	r.Close()
	// Second call must not panic (context already cancelled).
	r.Close()
}

// ─── OutlierDetector ──────────────────────────────────────────────────────────

func makeInstances(ids ...string) []*discovery.ServiceInstance {
	out := make([]*discovery.ServiceInstance, len(ids))
	for i, id := range ids {
		out[i] = &discovery.ServiceInstance{ID: id, Address: id + ":80"}
	}
	return out
}

func TestOutlierDetector_EjectsAfterThreshold(t *testing.T) {
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{
		ConsecutiveErrors: 3,
		EjectionInterval:  10 * time.Second,
		MaxEjectionPct:    100,
	})
	insts := makeInstances("a", "b", "c")

	bad := insts[0] // "a" will be ejected

	// 3 errors → ejection.
	od.RecordError(bad, time.Millisecond)
	od.RecordError(bad, time.Millisecond)
	od.RecordError(bad, time.Millisecond)

	// "a" must not be picked across many requests.
	for i := 0; i < 20; i++ {
		got, err := od.Pick(insts, "")
		testutil.AssertNoError(t, err)
		if got.ID == "a" {
			t.Fatalf("ejected instance 'a' was picked on attempt %d", i)
		}
	}
}

func TestOutlierDetector_SuccessResetsCounter(t *testing.T) {
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{
		ConsecutiveErrors: 3,
		EjectionInterval:  10 * time.Second,
	})
	insts := makeInstances("a", "b")
	bad := insts[0]

	// Two errors, then a success — counter resets, should not be ejected.
	od.RecordError(bad, time.Millisecond)
	od.RecordError(bad, time.Millisecond)
	od.RecordSuccess(bad, time.Millisecond)

	// "a" should still be selectable.
	seen := false
	for i := 0; i < 20; i++ {
		got, _ := od.Pick(insts, "")
		if got.ID == "a" {
			seen = true
			break
		}
	}
	if !seen {
		t.Error("expected 'a' to be selectable after RecordSuccess reset")
	}
}

func TestOutlierDetector_ReadmitsAfterInterval(t *testing.T) {
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{
		ConsecutiveErrors: 2,
		EjectionInterval:  50 * time.Millisecond, // very short for test
		MaxEjectionPct:    100,
	})
	insts := makeInstances("a", "b")
	bad := insts[0]

	od.RecordError(bad, time.Millisecond)
	od.RecordError(bad, time.Millisecond)

	// Verify ejected immediately.
	for i := 0; i < 10; i++ {
		got, _ := od.Pick(insts, "")
		if got.ID == "a" {
			t.Fatal("expected 'a' to be ejected")
		}
	}

	// Wait for readmission.
	time.Sleep(100 * time.Millisecond)

	seen := false
	for i := 0; i < 20; i++ {
		got, _ := od.Pick(insts, "")
		if got.ID == "a" {
			seen = true
			break
		}
	}
	if !seen {
		t.Error("expected 'a' to be readmitted after EjectionInterval")
	}
}

func TestOutlierDetector_FallbackWhenAllEjected(t *testing.T) {
	// If all instances are ejected, the detector must fall back to the full list
	// to prevent a total blackout.
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{
		ConsecutiveErrors: 1,
		EjectionInterval:  time.Hour,
		MaxEjectionPct:    100,
	})
	insts := makeInstances("a", "b", "c")

	for _, inst := range insts {
		od.RecordError(inst, time.Millisecond)
	}

	// Even with all ejected, Pick must not return ErrNoInstances.
	got, err := od.Pick(insts, "")
	testutil.AssertNoError(t, err)
	if got == nil {
		t.Fatal("fallback to full list: expected non-nil instance")
	}
}

func TestOutlierDetector_MaxEjectionPct(t *testing.T) {
	// With MaxEjectionPct = 50 and 4 instances, at most 2 can be ejected.
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{
		ConsecutiveErrors: 1,
		EjectionInterval:  time.Hour,
		MaxEjectionPct:    50,
	})
	insts := makeInstances("a", "b", "c", "d")

	// Eject all four.
	for _, inst := range insts {
		od.RecordError(inst, time.Millisecond)
	}

	// With 50% cap, at most 2 are ejected, so at least 2 pass through healthy().
	// Verify that Pick still returns results across many calls.
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		got, err := od.Pick(insts, "")
		testutil.AssertNoError(t, err)
		seen[got.ID] = true
	}
	if len(seen) < 2 {
		t.Errorf("MaxEjectionPct=50: expected ≥2 distinct instances, got %v", seen)
	}
}

func TestOutlierDetector_ImplementsReporter(t *testing.T) {
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{})
	var _ loadbalance.Reporter = od // compile-time check
}

func TestOutlierDetector_EjectedInstances(t *testing.T) {
	inner := loadbalance.NewRoundRobin()
	od := loadbalance.NewOutlierDetector(inner, loadbalance.OutlierConfig{
		ConsecutiveErrors: 2,
		EjectionInterval:  time.Hour,
	})
	insts := makeInstances("x", "y")

	od.RecordError(insts[0], time.Millisecond)
	od.RecordError(insts[0], time.Millisecond)

	ejected := od.EjectedInstances()
	if len(ejected) != 1 || ejected[0] != "x" {
		t.Errorf("EjectedInstances: want [x], got %v", ejected)
	}
}
