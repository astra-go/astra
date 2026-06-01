package astra

import (
	"testing"
)

// TestRunQUIC_WithoutImport verifies that calling RunQUIC without importing
// the quic package returns a clear error message.
func TestRunQUIC_WithoutImport(t *testing.T) {
	app := New()
	err := app.RunQUIC(":443", "cert.pem", "key.pem")
	if err == nil {
		t.Fatal("expected error when quic package not imported, got nil")
	}
	expected := "astra: RunQUIC requires importing github.com/astra-go/astra/quic"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

// TestRegisterQUICRunner verifies that the registration mechanism works correctly.
func TestRegisterQUICRunner(t *testing.T) {
	// Save original state
	original := quicRunner
	defer func() { quicRunner = original }()

	// Reset to nil to simulate unregistered state
	quicRunner = nil

	// Register a mock runner
	called := false
	mockRunner := func(app *App, addr, certFile, keyFile string) error {
		called = true
		if addr != ":443" {
			t.Errorf("expected addr :443, got %s", addr)
		}
		if certFile != "cert.pem" {
			t.Errorf("expected certFile cert.pem, got %s", certFile)
		}
		if keyFile != "key.pem" {
			t.Errorf("expected keyFile key.pem, got %s", keyFile)
		}
		return nil
	}

	RegisterQUICRunner(mockRunner)

	// Verify registration worked
	if quicRunner == nil {
		t.Fatal("RegisterQUICRunner did not set quicRunner")
	}

	// Call RunQUIC and verify the registered function is invoked
	app := New()
	err := app.RunQUIC(":443", "cert.pem", "key.pem")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("registered runner was not called")
	}
}
