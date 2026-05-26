package stream

import (
	"testing"
	"time"
)

// ─── msgRateLimiter ───────────────────────────────────────────────────────────

func TestMsgRateLimiter_ConsumesInitialBurst(t *testing.T) {
	rl := newMsgRateLimiter(1, 3) // burst=3, rate=1/s
	for i := 0; i < 3; i++ {
		if !rl.allow() {
			t.Fatalf("call %d should be allowed (within burst)", i+1)
		}
	}
	if rl.allow() {
		t.Fatal("4th call should be rejected — burst exhausted")
	}
}

func TestMsgRateLimiter_RefillsOverTime(t *testing.T) {
	rl := newMsgRateLimiter(200, 1) // 200 tokens/s → ~5ms per token
	if !rl.allow() {
		t.Fatal("initial call should be allowed")
	}
	if rl.allow() {
		t.Fatal("immediate second call should be rejected")
	}
	time.Sleep(15 * time.Millisecond) // should have ~3 tokens by now
	if !rl.allow() {
		t.Fatal("call after sleep should be allowed — tokens refilled")
	}
}

func TestMsgRateLimiter_BurstCap(t *testing.T) {
	rl := newMsgRateLimiter(1000, 2) // burst=2, very fast refill
	// Drain the burst
	rl.allow()
	rl.allow()
	// Sleep long enough to overshoot the burst ceiling
	time.Sleep(20 * time.Millisecond)
	// Should still be capped at burst (2)
	if !rl.allow() {
		t.Fatal("first call after sleep should be allowed")
	}
	if !rl.allow() {
		t.Fatal("second call should be allowed (burst=2)")
	}
	if rl.allow() {
		t.Fatal("third call should be rejected — burst ceiling is 2")
	}
}

// ─── connLimiter ───────────────────────────────────────────────────────────���─

func TestConnLimiter_AllowsUpToMax(t *testing.T) {
	cl := newConnLimiter(2)

	ok1, rel1 := cl.acquire("10.0.0.1:1001")
	if !ok1 {
		t.Fatal("first connection should be allowed")
	}

	ok2, rel2 := cl.acquire("10.0.0.1:1002")
	if !ok2 {
		t.Fatal("second connection should be allowed")
	}

	ok3, _ := cl.acquire("10.0.0.1:1003")
	if ok3 {
		t.Fatal("third connection should be rejected (limit=2)")
	}

	// Release one slot and verify a new connection is accepted.
	rel1()
	ok4, rel4 := cl.acquire("10.0.0.1:1004")
	if !ok4 {
		t.Fatal("connection should be allowed after a slot was released")
	}

	rel2()
	rel4()
}

func TestConnLimiter_DifferentIPsAreIndependent(t *testing.T) {
	cl := newConnLimiter(1)

	ok1, rel1 := cl.acquire("192.168.1.1:5000")
	if !ok1 {
		t.Fatal("first IP: first connection should be allowed")
	}
	defer rel1()

	ok2, rel2 := cl.acquire("192.168.1.2:5000")
	if !ok2 {
		t.Fatal("different IP should be allowed independently")
	}
	defer rel2()
}

func TestConnLimiter_ReleaseCleansUpEntry(t *testing.T) {
	cl := newConnLimiter(1)
	_, release := cl.acquire("127.0.0.1:9000")
	release()

	// Entry should have been removed; acquiring again must succeed.
	ok, rel := cl.acquire("127.0.0.1:9001")
	if !ok {
		t.Fatal("should be allowed after full release")
	}
	rel()

	cl.mu.Lock()
	count := len(cl.conns)
	cl.mu.Unlock()
	if count != 0 {
		t.Fatalf("conns map should be empty after release; got %d entries", count)
	}
}

// ─── hostOnly ────────────────────────────────────────────────────────────��───

func TestHostOnly(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:9090", "::1"},
		{"10.0.0.5:443", "10.0.0.5"},
		{"not-an-addr", "not-an-addr"},
	}
	for _, tc := range tests {
		if got := hostOnly(tc.addr); got != tc.want {
			t.Errorf("hostOnly(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

// ─── ErrRateLimited is exported ──────────────────────────────────────────────

func TestErrRateLimited_IsExported(t *testing.T) {
	if ErrRateLimited == nil {
		t.Fatal("ErrRateLimited must not be nil")
	}
	if ErrRateLimited.Error() == "" {
		t.Fatal("ErrRateLimited.Error() must return a non-empty string")
	}
}
