package quic

import (
	"crypto/tls"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// QUICOptions holds configuration for the HTTP/3 server.
type QUICOptions struct {
	// TLSAddr is the address for the companion TLS/HTTP1.1 server that
	// advertises HTTP/3 via Alt-Svc. Defaults to the same value as the
	// QUIC addr when empty (single-port mode).
	TLSAddr string

	// TLSConfig overrides the default TLS configuration.
	// When nil, a secure default is used: TLS 1.3 minimum (required by HTTP/3).
	TLSConfig *tls.Config

	// Allow0RTT enables QUIC 0-RTT early data for reconnecting clients.
	// Reduces latency on repeat connections at the cost of replay-attack
	// exposure — only enable on idempotent endpoints.
	// Default: false.
	Allow0RTT bool

	// MaxIdleTimeout is the maximum duration a QUIC connection may be idle
	// before the server closes it. Default: 30s.
	MaxIdleTimeout time.Duration

	// MaxIncomingStreams is the maximum number of concurrent incoming
	// bidirectional streams per QUIC connection. Default: 100.
	MaxIncomingStreams int64

	// AltSvcMaxAge is the max-age value (in seconds) for the Alt-Svc header.
	// Default: 86400 (24 h).
	AltSvcMaxAge int

	// MetricsProvider enables QUIC-layer OTel metrics (active connections,
	// handshake duration, 0-RTT hits, path migration events).
	// When nil, QUIC-layer metrics are disabled.
	// Use go.opentelemetry.io/otel.GetMeterProvider() to wire into the global
	// provider set up by observability.Module or otel.Setup.
	MetricsProvider metric.MeterProvider

	// TracerProvider enables per-connection OTel spans for QUIC connections.
	// When nil, QUIC-layer tracing is disabled.
	TracerProvider trace.TracerProvider
}

// QUICOption is a functional option for QUICOptions.
type QUICOption func(*QUICOptions)

// WithTLSAddr sets a separate listen address for the companion TLS server.
// Use this when the QUIC and TLS servers must bind to different ports.
func WithTLSAddr(addr string) QUICOption {
	return func(o *QUICOptions) { o.TLSAddr = addr }
}

// WithQUICSConfig sets a custom TLS configuration for both servers.
// The config must allow TLS 1.3 — HTTP/3 requires it.
func WithQUICSConfig(cfg *tls.Config) QUICOption {
	return func(o *QUICOptions) { o.TLSConfig = cfg }
}

// WithAllow0RTT enables or disables QUIC 0-RTT early data.
func WithAllow0RTT(allow bool) QUICOption {
	return func(o *QUICOptions) { o.Allow0RTT = allow }
}

// WithMaxIdleTimeout sets the QUIC connection idle timeout.
func WithMaxIdleTimeout(d time.Duration) QUICOption {
	return func(o *QUICOptions) { o.MaxIdleTimeout = d }
}

// WithMaxIncomingStreams sets the maximum concurrent incoming streams per connection.
func WithMaxIncomingStreams(n int64) QUICOption {
	return func(o *QUICOptions) { o.MaxIncomingStreams = n }
}

// WithAltSvcMaxAge sets the Alt-Svc max-age in seconds.
func WithAltSvcMaxAge(seconds int) QUICOption {
	return func(o *QUICOptions) { o.AltSvcMaxAge = seconds }
}

// WithQUICMetricsProvider enables QUIC-layer OTel metrics using mp.
// Pass otel.GetMeterProvider() to reuse the provider set up by observability.Module.
func WithQUICMetricsProvider(mp metric.MeterProvider) QUICOption {
	return func(o *QUICOptions) { o.MetricsProvider = mp }
}

// WithQUICTracerProvider enables per-connection OTel spans using tp.
// Pass otel.GetTracerProvider() to reuse the provider set up by observability.Module.
func WithQUICTracerProvider(tp trace.TracerProvider) QUICOption {
	return func(o *QUICOptions) { o.TracerProvider = tp }
}

func defaultQUICOptions() *QUICOptions {
	return &QUICOptions{
		MaxIdleTimeout:    30 * time.Second,
		MaxIncomingStreams: 100,
		AltSvcMaxAge:      86400,
	}
}

// defaultTLSConfig returns a TLS 1.3-only config suitable for HTTP/3.
func defaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
	}
}
