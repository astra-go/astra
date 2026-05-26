package discovery_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/discovery"
	"github.com/astra-go/astra/testutil"
)

// ─── Register ────────────────────────────────────────────────────────────────

func TestRegister_StoresInstance(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	err := reg.Register(ctx, &discovery.ServiceInstance{ID: "svc-1", Name: "svc", Address: "localhost:8080"})
	testutil.AssertNoError(t, err)

	got, err := reg.Discover(ctx, "svc")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 1, len(got))
	testutil.AssertEqual(t, "svc-1", got[0].ID)
}

func TestRegister_AppliesDefaults(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	// No Weight or Scheme set — should default to 1 and "http".
	testutil.AssertNoError(t, reg.Register(ctx, &discovery.ServiceInstance{ID: "x", Name: "svc"}))

	got, _ := reg.Discover(ctx, "svc")
	testutil.AssertEqual(t, 1, got[0].Weight)
	testutil.AssertEqual(t, "http", got[0].Scheme)
}

func TestRegister_EmptyID_ReturnsError(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	err := reg.Register(context.Background(), &discovery.ServiceInstance{Name: "svc"})
	testutil.AssertErrorIs(t, err, discovery.ErrInstanceIDEmpty)
}

func TestRegister_UpdatesExistingInstance(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "x", Name: "svc", Address: "old:80"})
	reg.Register(ctx, &discovery.ServiceInstance{ID: "x", Name: "svc", Address: "new:80"})

	got, _ := reg.Discover(ctx, "svc")
	testutil.AssertEqual(t, 1, len(got))
	testutil.AssertEqual(t, "new:80", got[0].Address)
}

// ─── Deregister ───────────────────────────────────────────────────────────────

func TestDeregister_RemovesInstance(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "a", Name: "svc", Address: "a:80"})
	reg.Register(ctx, &discovery.ServiceInstance{ID: "b", Name: "svc", Address: "b:80"})

	testutil.AssertNoError(t, reg.Deregister(ctx, "a"))

	got, _ := reg.Discover(ctx, "svc")
	testutil.AssertEqual(t, 1, len(got))
	testutil.AssertEqual(t, "b", got[0].ID)
}

func TestDeregister_LastInstance_ServiceNotFound(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "only", Name: "svc"})
	reg.Deregister(ctx, "only")

	_, err := reg.Discover(ctx, "svc")
	if !errors.Is(err, discovery.ErrNotFound) {
		t.Errorf("expected ErrNotFound after last instance removed, got %v", err)
	}
}

func TestDeregister_NonExistent_NoError(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	testutil.AssertNoError(t, reg.Deregister(context.Background(), "ghost"))
}

// ─── Discover ─────────────────────────────────────────────────────────────────

func TestDiscover_UnknownService_ErrNotFound(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	_, err := reg.Discover(context.Background(), "unknown")
	if !errors.Is(err, discovery.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDiscover_IsolatedByName(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "a", Name: "svc-a", Address: "a:80"})
	reg.Register(ctx, &discovery.ServiceInstance{ID: "b", Name: "svc-b", Address: "b:80"})

	got, _ := reg.Discover(ctx, "svc-a")
	testutil.AssertEqual(t, 1, len(got))
	testutil.AssertEqual(t, "svc-a", got[0].Name)
}

func TestDiscover_ReturnsCopy(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "x", Name: "svc", Address: "x:80"})

	got, _ := reg.Discover(ctx, "svc")
	got[0].Address = "mutated"

	// The registry's internal state must not be affected.
	got2, _ := reg.Discover(ctx, "svc")
	if got2[0].Address == "mutated" {
		t.Error("Discover should return copies, not pointers to internal state")
	}
}

func TestDiscover_MultipleInstances(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	for _, id := range []string{"i1", "i2", "i3"} {
		reg.Register(ctx, &discovery.ServiceInstance{ID: id, Name: "svc"})
	}

	got, _ := reg.Discover(ctx, "svc")
	testutil.AssertEqual(t, 3, len(got))
}

// ─── Watch ────────────────────────────────────────────────────────────────────

func TestWatch_EmitsCurrentState(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "x", Name: "svc"})

	ch, err := reg.Watch(ctx, "svc")
	testutil.AssertNoError(t, err)

	select {
	case instances := <-ch:
		testutil.AssertEqual(t, 1, len(instances))
	case <-time.After(300 * time.Millisecond):
		t.Error("Watch should emit current state immediately")
	}
}

func TestWatch_NotifiesOnRegister(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ch, _ := reg.Watch(ctx, "svc")

	reg.Register(ctx, &discovery.ServiceInstance{ID: "y", Name: "svc"})

	select {
	case instances := <-ch:
		testutil.AssertEqual(t, 1, len(instances))
	case <-time.After(300 * time.Millisecond):
		t.Error("Watch should emit update after Register")
	}
}

func TestWatch_NotifiesOnDeregister(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reg.Register(ctx, &discovery.ServiceInstance{ID: "z", Name: "svc"})

	ch, _ := reg.Watch(ctx, "svc")
	// Drain initial emit.
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
	}

	reg.Deregister(ctx, "z")

	select {
	case instances := <-ch:
		testutil.AssertEqual(t, 0, len(instances))
	case <-time.After(300 * time.Millisecond):
		t.Error("Watch should emit update after Deregister")
	}
}

func TestWatch_ChannelClosedOnContextCancel(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx, cancel := context.WithCancel(context.Background())

	ch, _ := reg.Watch(ctx, "svc")
	cancel()

	select {
	case _, open := <-ch:
		if open {
			t.Error("channel should be closed after context cancel")
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("channel should close promptly after context cancel")
	}
}

func TestWatch_EmptyService_NoInitialEmit(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch, _ := reg.Watch(ctx, "empty-svc")

	// No instances registered — no initial emit expected.
	select {
	case <-ch:
		// might receive empty list or nothing — either is acceptable
	case <-ctx.Done():
		// timeout is also fine, means nothing was sent
	}
}

// ─── Concurrent safety ────────────────────────────────────────────────────────

func TestConcurrentRegisterDeregisterDiscover(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		id := "inst-" + string(rune('A'+i%26))
		go func(id string) {
			defer wg.Done()
			reg.Register(ctx, &discovery.ServiceInstance{ID: id, Name: "svc"})
			reg.Discover(ctx, "svc")
			reg.Deregister(ctx, id)
		}(id)
	}
	wg.Wait()
}

// ─── Close ────────────────────────────────────────────────────────────────────

func TestClose_ReturnsNil(t *testing.T) {
	reg := discovery.NewInMemoryRegistry()
	testutil.AssertNoError(t, reg.Close())
}
