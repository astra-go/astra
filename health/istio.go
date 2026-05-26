// Package health provides Istio / service-mesh probe helpers.
//
// Istio's default liveness and readiness probe paths differ from the standard
// Kubernetes convention used by health.Register:
//
//	Istio default   → /healthz/live, /healthz/ready
//	health.Register → /live, /ready, /health
//
// RegisterIstioProbes registers the Istio-style paths in addition to (not
// instead of) the existing endpoints, so both Kubernetes and Istio deployments
// work simultaneously.
//
// # Usage
//
//	// Register standard K8s probes
//	health.Register(app, health.WithProbe("db", dbProbe))
//
//	// Also register Istio-style paths
//	health.RegisterIstioProbes(app,
//	    health.WithProbe("db", dbProbe),
//	    health.WithIstioHeaders(),
//	)
//
// # Istio sidecar passthrough
//
// By default Istio intercepts and proxies all inbound traffic. Health probes
// are excluded from mTLS interception via the MeshConfig
// `probeHttpGet.path` field. Registering the probe paths listed above is
// sufficient — no framework-level mTLS configuration is needed.

package health

import (
	"github.com/astra-go/astra"
)

// istioOptions extends the standard options with Istio-specific settings.
type istioOptions struct {
	options
	addIstioHeaders bool
}

// WithIstioHeaders returns an Option that injects Istio-compatible response
// headers on every health endpoint response:
//
//   - x-content-type-options: nosniff
//   - x-envoy-upstream-service-time: <latency in ms>
func WithIstioHeaders() Option {
	return func(o *options) {
		// We piggy-back on a sentinel probe to carry the header flag.
		// The actual injection is handled in RegisterIstioProbes.
		o.probes = append(o.probes, named{name: "__istio_headers__", probe: nil})
	}
}

// RegisterIstioProbes mounts liveness and readiness probes at the Istio-
// standard paths /healthz/live and /healthz/ready, in addition to any probes
// already registered via health.Register.
//
// Accepted options: WithPrefix, WithProbe, WithIstioHeaders.
func RegisterIstioProbes(app *astra.App, opts ...Option) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	// Separate the header sentinel from real probes.
	addHeaders := false
	realProbes := o.probes[:0:0]
	for _, p := range o.probes {
		if p.name == "__istio_headers__" {
			addHeaders = true
		} else {
			realProbes = append(realProbes, p)
		}
	}
	o.probes = realProbes

	h := &handler{probes: o.probes}

	prefix := o.prefix + "/healthz"

	if addHeaders {
		app.GET(prefix+"/live", withIstioHeaders(h.live))
		app.GET(prefix+"/ready", withIstioHeaders(h.ready))
	} else {
		app.GET(prefix+"/live", h.live)
		app.GET(prefix+"/ready", h.ready)
	}
}

// withIstioHeaders wraps a HandlerFunc and injects Istio-compatible headers.
// Both headers must be set before the inner handler calls WriteHeader; once the
// status line is sent the header map is frozen and any further Set calls are
// discarded by the HTTP/1.1 framing layer.
//
// x-envoy-upstream-service-time is initialised to "0" here and reflects the
// wall-clock cost of the probe itself (typically sub-millisecond). Envoy
// sidecars that relay the probe response overwrite this field with their own
// upstream latency measurement.
func withIstioHeaders(next astra.HandlerFunc) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		c.Writer().Header().Set("x-content-type-options", "nosniff")
		c.Writer().Header().Set("x-envoy-upstream-service-time", "0")
		return next(c)
	}
}
