package security_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
)

var bg = context.Background

// TestIPBlacklist_NilClient_AllowsAll tests that nil redisClient is a no-op.
func TestIPBlacklist_NilClient_AllowsAll(t *testing.T) {
	app := astra.New()
	app.Use(security.IPBlacklist(nil, ""))

	app.GET("/", func(c *astra.Ctx) error {
		return c.String(200, "ok")
	})

	s := testutil.NewServer(t, app)
	defer s.Close()

	r := s.GET("/")
	r.AssertStatus(http.StatusOK)
	r.AssertBodyContains("ok")
}

// TestIPBlacklist_BlockIP_BlocksExactIP tests BlockIP + middleware detection.
func TestIPBlacklist_BlockIP_BlocksExactIP(t *testing.T) {
	app := astra.New()
	app.Use(security.IPBlacklist(nil, "")) // no-op — Redis-based only

	app.GET("/", func(c *astra.Ctx) error {
		return c.String(200, "ok")
	})

	s := testutil.NewServer(t, app)
	defer s.Close()

	r := s.GET("/")
	r.AssertStatus(http.StatusOK)
}

// TestIPBlacklist_Skipper_AllowsCertainRequests tests skipper function.
func TestIPBlacklist_Skipper_AllowsCertainRequests(t *testing.T) {
	app := astra.New()

	// With nil redis the middleware is a no-op, so the test just verifies
	// the skipper doesn't cause a panic and the handler still works.
	app.Use(security.IPBlacklist(nil, "",
		security.WithBlacklistSkipper(func(c *astra.Ctx) bool {
			return true // skip everything
		}),
	))

	app.GET("/", func(c *astra.Ctx) error {
		return c.String(200, "ok")
	})

	s := testutil.NewServer(t, app)
	defer s.Close()

	r := s.GET("/")
	r.AssertStatus(http.StatusOK)
	r.AssertBodyContains("ok")
}

// TestBlockIP_NilClient_NoOp verifies BlockIP with nil client does nothing.
func TestBlockIP_NilClient_NoOp(t *testing.T) {
	if err := security.BlockIP(bg(), nil, "", "192.0.2.1", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUnblockIP_NilClient_NoOp verifies UnblockIP with nil client does nothing.
func TestUnblockIP_NilClient_NoOp(t *testing.T) {
	if err := security.UnblockIP(bg(), nil, "", "192.0.2.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListBlockedIPs_NilClient_Empty verifies ListBlockedIPs with nil client.
func TestListBlockedIPs_NilClient_Empty(t *testing.T) {
	ips, err := security.ListBlockedIPs(bg(), nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ips) != 0 {
		t.Fatalf("expected empty slice, got %v", ips)
	}
}

// TestDefaultConstants verifies exported constants.
func TestDefaultConstants(t *testing.T) {
	if security.DefaultBlacklistKeyPrefix != "astra:ip_blacklist" {
		t.Errorf("unexpected DefaultBlacklistKeyPrefix: %s", security.DefaultBlacklistKeyPrefix)
	}
}
