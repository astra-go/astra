// Package middleware — IP address allow-list / block-list filter.
//
// Evaluates the real client IP against configurable CIDR rules before the
// request reaches any handler. Rules can be provided statically at startup
// or refreshed at runtime via a custom Loader function.
//
// # Static rules
//
//	app.Use(middleware.IPFilter(middleware.IPFilterConfig{
//	    // Allow only private network + a specific CDN range
//	    Allowlist: []string{
//	        "10.0.0.0/8",
//	        "172.16.0.0/12",
//	        "192.168.0.0/16",
//	        "203.0.113.0/24",
//	    },
//	    // Blocklist takes precedence over allowlist
//	    Blocklist: []string{"10.10.0.5/32"},
//	}))
//
// # Dynamic rules (reload from DB / config)
//
//	app.Use(middleware.IPFilter(middleware.IPFilterConfig{
//	    Loader: func(ctx context.Context) (allow, block []string, err error) {
//	        return db.LoadIPRules(ctx)
//	    },
//	    ReloadInterval: 5 * time.Minute,
//	}))
//
// # IP extraction
//
// By default the middleware reads X-Forwarded-For → X-Real-IP → RemoteAddr,
// matching the same priority chain as the audit-log middleware.
// Override with a custom GetIP function when running behind a trusted proxy.
package security

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// IPFilterConfig configures the IP filter middleware.
type IPFilterConfig struct {
	// Allowlist is a list of CIDR strings. When non-empty, only IPs that
	// match at least one CIDR are permitted. Evaluated AFTER Blocklist.
	Allowlist []string

	// Blocklist is a list of CIDR strings. Matching IPs are always rejected,
	// regardless of Allowlist. Evaluated BEFORE Allowlist.
	Blocklist []string

	// Loader, when set, is called at startup and then every ReloadInterval to
	// refresh the allow/block lists. The returned slices replace the in-memory lists.
	Loader func(ctx context.Context) (allow, block []string, err error)

	// ReloadInterval controls how often Loader is called. Default: 5 minutes.
	ReloadInterval time.Duration

	// ReloadCancel, when non-nil, is called to stop the background reload goroutine.
	// Typically set to the cancel function returned by context.WithCancel.
	// If nil, the reload goroutine runs until the process exits.
	ReloadCancel context.CancelFunc

	// TrustedProxies is a list of trusted proxy IP/CIDR strings.
	// When non-empty, X-Forwarded-For is only trusted when the immediate
	// client (RemoteAddr) matches a trusted proxy. When empty (default),
	// X-Forwarded-For is used as-is (compatible with current behavior but
	// insecure if the service is directly exposed to untrusted clients).
	//
	// Example: []string{"10.0.0.0/8", "172.16.0.0/12"} to trust internal proxies.
	TrustedProxies []string

	// DenyStatus is the HTTP status code returned when a request is rejected.
	// Default: 403 Forbidden.
	DenyStatus int

	// GetIP extracts the real client IP from the request context.
	// Default: X-Forwarded-For → X-Real-IP → RemoteAddr.
	GetIP func(c *astra.Ctx) string

	// Skipper skips filtering for matching requests.
	Skipper Skipper
}

// IPFilter returns an IP allow-list / block-list middleware.
func IPFilter(cfg IPFilterConfig) astra.HandlerFunc {
	if cfg.DenyStatus == 0 {
		cfg.DenyStatus = http.StatusForbidden
	}
	if cfg.GetIP == nil {
		cfg.GetIP = func(c *astra.Ctx) string { return realClientIP(c.Request()) }
	}
	if cfg.ReloadInterval == 0 {
		cfg.ReloadInterval = 5 * time.Minute
	}

	state := &ipFilterState{}
	state.update(cfg.Allowlist, cfg.Blocklist)

	// Parse trusted proxy CIDRs for X-Forwarded-For validation.
	trustedNets := parseCIDRs(cfg.TrustedProxies)

	// Start background reload if Loader is provided.
	reloadCtx, reloadCancel := context.WithCancel(context.Background())
	if cfg.Loader != nil {
		// Initial load.
		if allow, block, err := cfg.Loader(reloadCtx); err == nil {
			state.update(allow, block)
		}
		go func() {
			t := time.NewTicker(cfg.ReloadInterval)
			defer t.Stop()
			for {
				select {
				case <-reloadCtx.Done():
					return
				case <-t.C:
					allow, block, err := cfg.Loader(reloadCtx)
					if err == nil {
						state.update(allow, block)
					}
				}
			}
		}()
	}

	// Expose reload cancel so callers can stop the background goroutine.
	if cfg.ReloadCancel == nil {
		cfg.ReloadCancel = reloadCancel
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		ip := cfg.GetIP(c)
		// When TrustedProxies is configured, validate that X-Forwarded-For
		// comes from a trusted proxy. If not, fall back to RemoteAddr.
		if len(trustedNets) > 0 {
			ip = extractTrustedIP(c.Request(), trustedNets)
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			// Unparseable IP — deny by default (fail-safe).
			return c.JSON(cfg.DenyStatus, map[string]any{"error": "forbidden"})
		}

		if !state.allowed(parsed) {
			return c.JSON(cfg.DenyStatus, map[string]any{"error": "forbidden"})
		}

		c.Next()
		return nil
	}
}

// ─── internal state ────────────────────────────────────────────────────────

type ipFilterState struct {
	mu        sync.RWMutex
	allowNets []*net.IPNet
	blockNets []*net.IPNet
}

func (s *ipFilterState) update(allowList, blockList []string) {
	allow := parseCIDRs(allowList)
	block := parseCIDRs(blockList)
	s.mu.Lock()
	s.allowNets = allow
	s.blockNets = block
	s.mu.Unlock()
}

// allowed returns true when the IP should be permitted:
//  1. Denied if it matches any block CIDR.
//  2. Permitted if allowlist is empty (no restriction) OR IP matches an allow CIDR.
//  3. Denied otherwise.
func (s *ipFilterState) allowed(ip net.IP) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, n := range s.blockNets {
		if n.Contains(ip) {
			return false
		}
	}
	if len(s.allowNets) == 0 {
		return true
	}
	for _, n := range s.allowNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func parseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}

// realClientIP extracts the best-guess real client IP.
func realClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}
	return remoteAddrHost(r)
}

// extractTrustedIP returns the real client IP when TrustedProxies is configured.
// It validates that the immediate client (RemoteAddr) is a trusted proxy before
// trusting X-Forwarded-For or X-Real-IP. Falls back to RemoteAddr otherwise.
func extractTrustedIP(r *http.Request, trustedNets []*net.IPNet) string {
	host := remoteAddrHost(r)
	parsedRemote := net.ParseIP(host)
	if parsedRemote == nil {
		return host
	}

	// Check if the immediate client is a trusted proxy.
	trusted := false
	for _, n := range trustedNets {
		if n.Contains(parsedRemote) {
			trusted = true
			break
		}
	}

	if trusted {
		// Remote is a trusted proxy — trust X-Forwarded-For / X-Real-IP.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if net.ParseIP(xri) != nil {
				return xri
			}
		}
	}

	// Not a trusted proxy or no forwarding headers — use RemoteAddr.
	return host
}

// remoteAddrHost extracts the host portion from RemoteAddr.
func remoteAddrHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
