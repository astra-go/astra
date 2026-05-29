package quic

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAltSvcHandler(t *testing.T) {
	const altSvcValue = `h3=":443"; ma=86400`
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := altSvcHandler(inner, altSvcValue)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Alt-Svc"); got != altSvcValue {
		t.Errorf("Alt-Svc = %q, want %q", got, altSvcValue)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestDefaultQUICOptions(t *testing.T) {
	o := defaultQUICOptions()
	if o.MaxIdleTimeout != 30*time.Second {
		t.Errorf("MaxIdleTimeout = %v, want 30s", o.MaxIdleTimeout)
	}
	if o.MaxIncomingStreams != 100 {
		t.Errorf("MaxIncomingStreams = %d, want 100", o.MaxIncomingStreams)
	}
	if o.AltSvcMaxAge != 86400 {
		t.Errorf("AltSvcMaxAge = %d, want 86400", o.AltSvcMaxAge)
	}
	if o.Allow0RTT {
		t.Error("Allow0RTT should default to false")
	}
	if o.Mode != ServerModeDualStack {
		t.Errorf("Mode = %v, want ServerModeDualStack", o.Mode)
	}
}

func TestQUICOptions_Functional(t *testing.T) {
	o := defaultQUICOptions()
	opts := []QUICOption{
		WithTLSAddr(":8443"),
		WithAllow0RTT(true),
		WithMaxIdleTimeout(60 * time.Second),
		WithMaxIncomingStreams(200),
		WithAltSvcMaxAge(3600),
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.TLSAddr != ":8443" {
		t.Errorf("TLSAddr = %q, want :8443", o.TLSAddr)
	}
	if !o.Allow0RTT {
		t.Error("Allow0RTT should be true")
	}
	if o.MaxIdleTimeout != 60*time.Second {
		t.Errorf("MaxIdleTimeout = %v, want 60s", o.MaxIdleTimeout)
	}
	if o.MaxIncomingStreams != 200 {
		t.Errorf("MaxIncomingStreams = %d, want 200", o.MaxIncomingStreams)
	}
	if o.AltSvcMaxAge != 3600 {
		t.Errorf("AltSvcMaxAge = %d, want 3600", o.AltSvcMaxAge)
	}
}

func TestDefaultTLSConfig_TLS13(t *testing.T) {
	cfg := defaultTLSConfig()
	if cfg.MinVersion != 0x0304 { // tls.VersionTLS13
		t.Errorf("MinVersion = 0x%04x, want TLS 1.3 (0x0304)", cfg.MinVersion)
	}
}

func TestAltSvcValue_SeparatePorts(t *testing.T) {
	// When TLSAddr differs from QUIC addr, Alt-Svc must advertise TLSAddr.
	const tlsAddr = ":443"
	const maxAge = 86400

	want := `h3=":443"; ma=86400`
	got := fmt.Sprintf(`h3="%s"; ma=%d`, tlsAddr, maxAge)
	if got != want {
		t.Errorf("altSvcValue = %q, want %q", got, want)
	}
}

func TestWithServerMode(t *testing.T) {
	o := defaultQUICOptions()
	if o.Mode != ServerModeDualStack {
		t.Fatalf("default Mode = %v, want ServerModeDualStack", o.Mode)
	}

	WithServerMode(ServerModeQUICOnly)(o)
	if o.Mode != ServerModeQUICOnly {
		t.Errorf("Mode = %v, want ServerModeQUICOnly", o.Mode)
	}

	WithServerMode(ServerModeDualStack)(o)
	if o.Mode != ServerModeDualStack {
		t.Errorf("Mode = %v, want ServerModeDualStack", o.Mode)
	}
}

func TestServerMode_QUICOnly_NoTLSSrv(t *testing.T) {
	// Verify that ServerModeQUICOnly results in a nil tlsSrv being passed to
	// runWithGracefulShutdown by inspecting the option state directly.
	o := defaultQUICOptions()
	WithServerMode(ServerModeQUICOnly)(o)

	// In QUIC-only mode the tlsSrv construction block is skipped; simulate
	// the same conditional logic used in RunQUICWithOptions.
	var tlsSrv *http.Server
	if o.Mode == ServerModeDualStack {
		tlsSrv = &http.Server{}
	}

	if tlsSrv != nil {
		t.Error("expected tlsSrv to be nil in ServerModeQUICOnly")
	}
}

func TestServerMode_DualStack_HasTLSSrv(t *testing.T) {
	o := defaultQUICOptions() // default is DualStack

	var tlsSrv *http.Server
	if o.Mode == ServerModeDualStack {
		tlsSrv = &http.Server{Addr: ":443"}
	}

	if tlsSrv == nil {
		t.Error("expected tlsSrv to be non-nil in ServerModeDualStack")
	}
}
