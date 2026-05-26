package astra_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra"
)

func TestLifecycle_RunStopHooks_LIFO(t *testing.T) {
	lc := &astra.Lifecycle{}
	var order []int

	lc.OnStop(func(_ context.Context) error { order = append(order, 1); return nil })
	lc.OnStop(func(_ context.Context) error { order = append(order, 2); return nil })
	lc.OnStop(func(_ context.Context) error { order = append(order, 3); return nil })

	lc.RunStopHooks(context.Background())

	want := []int{3, 2, 1}
	if len(order) != len(want) {
		t.Fatalf("got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("stop hook order: got %v, want %v", order, want)
		}
	}
}

func TestLifecycle_RunStopHooks_AllRunOnError(t *testing.T) {
	lc := &astra.Lifecycle{}
	ran := make([]bool, 3)

	lc.OnStop(func(_ context.Context) error { ran[0] = true; return nil })
	lc.OnStop(func(_ context.Context) error { ran[1] = true; return nil })
	lc.OnStop(func(_ context.Context) error { ran[2] = true; return nil })

	lc.RunStopHooks(context.Background())

	for i, r := range ran {
		if !r {
			t.Errorf("stop hook %d did not run", i)
		}
	}
}

func TestLifecycle_RunStartHooks_Order(t *testing.T) {
	lc := &astra.Lifecycle{}
	var order []int

	lc.OnStart(func(_ context.Context) error { order = append(order, 1); return nil })
	lc.OnStart(func(_ context.Context) error { order = append(order, 2); return nil })

	if err := lc.RunStartHooks(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("start hook order: got %v, want [1 2]", order)
	}
}
