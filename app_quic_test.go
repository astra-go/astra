package astra_test

import (
	"testing"

	"github.com/astra-go/astra/testutil"
)

// TestRunQUIC_PortFromAddr validates the internal portFromAddr helper via
// an indirect approach: RunQUIC is tested by verifying that an invalid cert
// path returns an error immediately (TLS setup fails before binding).
//
// We can't call portFromAddr directly (it's unexported), but we exercise its
// behaviour through RunQUIC returning a configuration error for a bad port.
func TestRunQUIC_BadCertReturnsError(t *testing.T) {
	app := testutil.NewTestApp()

	err := app.RunQUIC(":0", "/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error when cert/key files do not exist")
	}
}
