package astra

import (
	"log/slog"
	"net"

	"github.com/astra-go/astra/binding"
	"github.com/astra-go/astra/contract"
)

// Options holds all configuration for the App.
// Use the With* functional options to set fields; do not modify Options directly.
type Options struct {
	// MaxMultipartMemory is the max memory for multipart form parsing (default 32MB).
	MaxMultipartMemory int64
	// MaxJSONBodySize is the max bytes read from the request body in BindJSON
	// (default 1 MiB). Set to a larger value for endpoints that accept bulk JSON
	// payloads; set smaller to tighten limits for specific apps.
	MaxJSONBodySize int64
	// MaxXMLBodySize is the max bytes read from the request body in BindXML
	// (default 1 MiB). Set to a larger value for endpoints that accept bulk XML
	// payloads; set smaller to tighten limits for specific apps.
	MaxXMLBodySize int64
	// MaxMultipartMemory is the max memory for multipart form parsing (default 32MB).
	ErrorHandler ErrorHandler
	// NotFoundHandler is called when no route matches.
	NotFoundHandler HandlerFunc
	// MethodNotAllowedHandler is called when the method is not allowed.
	MethodNotAllowedHandler HandlerFunc
	// ShutdownTimeout is the graceful shutdown timeout in seconds. Default: 10.
	ShutdownTimeout int
	// TrustedProxies is a list of trusted proxy IP addresses or CIDR ranges.
	// E.g. ["10.0.0.0/8", "172.16.0.0/12", "127.0.0.1"].
	// Compiled into trustedNets once by New(); do not modify after construction.
	TrustedProxies []string
	// Mode controls error response verbosity. Default: ModeDev.
	Mode Mode
	// Serializer overrides the default encoding/json serializer.
	// Use faster alternatives (sonic, jsoniter) for high-throughput APIs.
	Serializer Serializer
	// Renderer is the server-side template engine used by c.Render.
	// Register a render.HTMLEngine (or any custom Renderer) via WithRenderer.
	// When nil, c.Render returns an error directing the caller to register one.
	Renderer Renderer
	// Binder is the request-data binding and validation implementation.
	// Defaults to binding.Default (go-playground/validator backed).
	// Replace with a custom contract.Binder to swap struct-tag conventions,
	// the validation library, or both.
	Binder contract.Binder

	// StrictConflict causes Add to panic on duplicate route registration instead
	// of warning and overwriting. Automatically enabled in ModeTest.
	// Set via WithStrictConflict for non-test environments.
	StrictConflict bool

	// customRouter, when non-nil, is used instead of newRouter(app).
	// Set via WithRouter. Unexported: callers interact only through the option.
	customRouter HttpRouter

	// trustedNets is the compiled form of TrustedProxies.
	// Populated once by prepareTrustedNets() in New(); never nil after that.
	trustedNets []*net.IPNet

	// MaxParamValueLen limits the byte length of a URL path parameter value.
	// Requests whose parameter segment exceeds this limit are treated as 404.
	// 0 (default) disables the limit.
	MaxParamValueLen int
}

// ─── Option constructors ──────────────────────────────────────────────────────

// WithMaxMultipartMemory sets the max memory for multipart form parsing.
func WithMaxMultipartMemory(size int64) Option {
	return func(o *Options) { o.MaxMultipartMemory = size }
}

// WithMaxJSONBodySize sets the maximum number of bytes read from the request
// body by BindJSON / ShouldBindJSON / MustBindJSON. The default is 1 MiB
// (1 << 20). Use a larger value for bulk-import endpoints and a smaller one
// when you want stricter request size limits.
func WithMaxJSONBodySize(size int64) Option {
	return func(o *Options) { o.MaxJSONBodySize = size }
}

// WithMaxXMLBodySize sets the maximum number of bytes read from the request
// body by BindXML / ShouldBindXML / MustBindXML. The default is 1 MiB
// (1 << 20). Use a larger value for bulk-import endpoints and a smaller one
// when you want stricter request size limits.
func WithMaxXMLBodySize(size int64) Option {
	return func(o *Options) { o.MaxXMLBodySize = size }
}

// WithErrorHandler sets the global error handler.
func WithErrorHandler(h ErrorHandler) Option {
	return func(o *Options) { o.ErrorHandler = h }
}

// WithShutdownTimeout sets the graceful shutdown timeout in seconds.
func WithShutdownTimeout(seconds int) Option {
	return func(o *Options) { o.ShutdownTimeout = seconds }
}

// WithTrustedProxies sets trusted proxy IPs for correct client IP detection.
// Each entry may be a plain IP ("192.168.1.1") or CIDR range ("10.0.0.0/8").
func WithTrustedProxies(proxies []string) Option {
	return func(o *Options) { o.TrustedProxies = proxies }
}

// WithMode sets the application run mode (dev, prod, staging, test).
func WithMode(m Mode) Option {
	return func(o *Options) { o.Mode = m }
}

// WithSerializer replaces the default encoding/json serializer.
//
//	import "github.com/bytedance/sonic"
//	app := astra.New(astra.WithSerializer(sonic.ConfigStd))
func WithSerializer(s Serializer) Option {
	return func(o *Options) { o.Serializer = s }
}

// WithRenderer sets the server-side template engine.
// Use the render sub-package for the built-in HTMLEngine:
//
//	import "github.com/astra-go/astra/render"
//
//	app := astra.New(astra.WithRenderer(render.Must(render.Config{
//	    Root:   "templates",
//	    Layout: "layouts/base.html",
//	})))
func WithRenderer(r Renderer) Option {
	return func(o *Options) { o.Renderer = r }
}

// WithBinder replaces the default binding and validation implementation.
// The default is binding.DefaultBinder (go-playground/validator backed).
// Use this to swap struct-tag conventions or the validation library.
func WithBinder(b contract.Binder) Option {
	return func(o *Options) { o.Binder = b }
}

// WithStrictConflict makes the router panic when a duplicate route is registered,
// instead of warning and silently overwriting. Automatically active in ModeTest.
func WithStrictConflict() Option {
	return func(o *Options) { o.StrictConflict = true }
}

// WithRouter replaces the default radix-tree router with a custom implementation.
// The supplied HttpRouter receives all route registrations via Add and dispatches
// incoming requests via Handle.  Use this for testing (pass a mock router) or to
// plug in a different routing algorithm.
func WithRouter(r HttpRouter) Option {
	return func(o *Options) { o.customRouter = r }
}

// WithMaxParamValueLen sets the maximum byte length for URL path parameter values.
// Requests whose parameter segment exceeds this limit are treated as 404.
// Pass 0 to disable the limit (default).
// Panics if n < 0.
func WithMaxParamValueLen(n int) Option {
	if n < 0 {
		panic("astra: WithMaxParamValueLen: n must be >= 0")
	}
	return func(o *Options) { o.MaxParamValueLen = n }
}

// WithNotFoundHandler sets a custom 404 handler.
func WithNotFoundHandler(h HandlerFunc) Option {
	return func(o *Options) { o.NotFoundHandler = h }
}

// WithMethodNotAllowedHandler sets a custom 405 handler.
func WithMethodNotAllowedHandler(h HandlerFunc) Option {
	return func(o *Options) { o.MethodNotAllowedHandler = h }
}

// ─── Options methods ──────────────────────────────────────────────────────────

// prepareTrustedNets compiles TrustedProxies strings into []*net.IPNet once at
// startup.  Both CIDR ranges ("10.0.0.0/8") and plain IPs ("127.0.0.1") are
// accepted; plain IPs are promoted to single-host CIDRs (/32 or /128).
// Invalid entries emit a slog.Warn and are skipped so a typo does not prevent
// startup, but the warning makes misconfiguration visible in logs.
func (o *Options) prepareTrustedNets() {
	o.trustedNets = make([]*net.IPNet, 0, len(o.TrustedProxies))
	for _, proxy := range o.TrustedProxies {
		if _, cidr, err := net.ParseCIDR(proxy); err == nil {
			o.trustedNets = append(o.trustedNets, cidr)
			continue
		}
		// Plain IP: promote to single-host CIDR so all lookups go through
		// the same net.IPNet.Contains path.
		if ip := net.ParseIP(proxy); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
				ip = ip.To4() // use 4-byte form to match ParseCIDR output
			}
			o.trustedNets = append(o.trustedNets, &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(bits, bits),
			})
			continue
		}
		slog.Warn("astra: invalid trusted proxy entry, skipping", "entry", proxy)
	}
}

// isTrustedProxy reports whether ip is covered by any pre-compiled trusted
// network.  The ip argument must already be parsed (never nil).
// O(n) in the number of trusted networks; typical deployments have < 10.
func (o *Options) isTrustedProxy(ip net.IP) bool {
	for _, cidr := range o.trustedNets {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// serializer returns the configured serializer or the default encoding/json one.
func (o *Options) serializer() Serializer {
	if o.Serializer != nil {
		return o.Serializer
	}
	return defaultSerializer
}

// ─── Defaults ─────────────────────────────────────────────────────────────────

func defaultOptions() *Options {
	return &Options{
		MaxMultipartMemory:      32 << 20, // 32 MB
		MaxJSONBodySize:         1 << 20,  // 1 MiB
		MaxXMLBodySize:          1 << 20,  // 1 MiB
		ShutdownTimeout:         10,
		MaxParamValueLen:        256,
		Mode:                    ModeDev,
		ErrorHandler:            defaultErrorHandler,
		NotFoundHandler:         defaultNotFoundHandler,
		MethodNotAllowedHandler: defaultMethodNotAllowedHandler,
		Binder:                  binding.Default,
	}
}

// slimDefaultOptions returns a minimal Options for NewSlim().
// Binder is intentionally nil: slim apps do not pull in go-playground/validator
// and therefore cannot use c.Bind / c.ShouldBind.  All other defaults match
// defaultOptions so routing, error handling, and shutdown behave identically.
func slimDefaultOptions() *Options {
	return &Options{
		MaxMultipartMemory:      32 << 20,
		MaxJSONBodySize:         1 << 20, // 1 MiB
		MaxXMLBodySize:          1 << 20,  // 1 MiB
		ShutdownTimeout:         10,
		Mode:                    ModeDev,
		ErrorHandler:            defaultErrorHandler,
		NotFoundHandler:         defaultNotFoundHandler,
		MethodNotAllowedHandler: defaultMethodNotAllowedHandler,
		// Binder: nil — binding/validation subsystem is disabled in slim mode.
	}
}
