package astra

// context_request.go — HTTP request-reading methods for Ctx.
//
// Covers raw request/writer accessors, URL path parameters, query parameters,
// form fields, headers, and client metadata (IP, User-Agent, WebSocket detection).
// All methods are read-only with respect to the HTTP request; mutation helpers
// (SetRequest, SetWriter, SetHeader) are included here for cohesion.

import (
	"mime/multipart"
	"net"
	"net/http"
	"strings"

	"github.com/astra-go/astra/contract"
)

// ─── Request / Response accessors ────────────────────────────────────────────

// Request returns the underlying *http.Request.
func (c *Ctx) Request() *http.Request { return c.req }

// SetRequest replaces the underlying HTTP request.
// Used by middleware (e.g. tracing) to attach a context-carrying request.
func (c *Ctx) SetRequest(r *http.Request) { c.req = r }

// Writer returns the enhanced response writer.
func (c *Ctx) Writer() contract.ResponseWriter { return c.writer }

// SetWriter replaces the response writer.
// Used by middleware (e.g. compress) to wrap the writer with a buffering layer.
func (c *Ctx) SetWriter(w contract.ResponseWriter) { c.writer = w }

// ─── Path Parameters ─────────────────────────────────────────────────────────

// Param returns the value of the URL path parameter by name.
func (c *Ctx) Param(key string) string {
	return c.params.ByName(key)
}

// ─── Query Parameters ────────────────────────────────────────────────────────

// queryFast scans rawQuery for key without calling url.ParseQuery.
// It is only valid when rawQuery contains no '%' or '+' characters
// (no percent- or plus-encoding). Returns ("", false) when key is absent.
func queryFast(rawQuery, key string) (string, bool) {
	for rawQuery != "" {
		var seg string
		if i := strings.IndexByte(rawQuery, '&'); i >= 0 {
			seg, rawQuery = rawQuery[:i], rawQuery[i+1:]
		} else {
			seg, rawQuery = rawQuery, ""
		}
		if i := strings.IndexByte(seg, '='); i >= 0 {
			if seg[:i] == key {
				return seg[i+1:], true
			}
		} else if seg == key {
			return "", true
		}
	}
	return "", false
}

// Query returns the URL query parameter by name.
// The query string is parsed once per request and cached; repeated calls
// for the same request are map lookups with no allocation.
func (c *Ctx) Query(key string) string {
	raw := c.req.URL.RawQuery
	if raw == "" {
		return ""
	}
	// Fast path: plain ASCII query strings (no % or + encoding) can be scanned
	// directly without ParseQuery — zero allocations for the common case.
	if !strings.ContainsAny(raw, "%+") {
		v, _ := queryFast(raw, key)
		return v
	}
	if c.queryCache == nil {
		c.queryCache = c.req.URL.Query()
	}
	return c.queryCache.Get(key)
}

// DefaultQuery returns the URL query parameter or a default value if missing.
func (c *Ctx) DefaultQuery(key, defaultValue string) string {
	v := c.Query(key)
	if v == "" {
		return defaultValue
	}
	return v
}

// QueryMap returns all query parameters as a map.
func (c *Ctx) QueryMap() map[string]string {
	if c.queryCache == nil {
		c.queryCache = c.req.URL.Query()
	}
	result := make(map[string]string, len(c.queryCache))
	for k, v := range c.queryCache {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// ─── Form Parameters ─────────────────────────────────────────────────────────

// PostForm returns the form value for the given key.
func (c *Ctx) PostForm(key string) string {
	return c.req.FormValue(key)
}

// DefaultPostForm returns the form value or a default if not present.
func (c *Ctx) DefaultPostForm(key, defaultValue string) string {
	v := c.PostForm(key)
	if v == "" {
		return defaultValue
	}
	return v
}

// FormFile returns the file header for the given multipart form key.
func (c *Ctx) FormFile(key string) (*multipart.FileHeader, error) {
	if err := c.req.ParseMultipartForm(c.app.options.MaxMultipartMemory); err != nil {
		return nil, err
	}
	_, fh, err := c.req.FormFile(key)
	return fh, err
}

// ─── Headers ─────────────────────────────────────────────────────────────────

// SetHeader sets a response header.
func (c *Ctx) SetHeader(key, value string) {
	c.writer.Header().Set(key, value)
}

// Header returns a request header value.
func (c *Ctx) Header(key string) string {
	return c.req.Header.Get(key)
}

// ContentType returns the Content-Type of the request.
func (c *Ctx) ContentType() string {
	ct := c.req.Header.Get("Content-Type")
	for i, char := range ct {
		if char == ' ' || char == ';' {
			return ct[:i]
		}
	}
	return ct
}

// ─── Client Info ─────────────────────────────────────────────────────────────

// ClientIP returns the real client IP.
//
// Proxy headers (X-Forwarded-For, X-Real-Ip) are only consulted when the
// direct peer (RemoteAddr) is listed in TrustedProxies; otherwise RemoteAddr
// is returned immediately, preventing header injection by an untrusted peer.
//
// X-Forwarded-For is built left-to-right as a request travels through proxies:
//
//	X-Forwarded-For: <client>, <proxy-1>, <proxy-2>
//
// The leftmost entry is whatever the original sender wrote — fully attacker-
// controlled.  Returning it directly (left-to-right traversal) allows a client
// to forge any IP by prepending it before the real address.
//
// Instead we walk right-to-left: the rightmost entries were appended by
// proxies we trust, so we skip them and return the first IP that did NOT come
// from a trusted proxy — that is the true client address.
//
// If every entry in XFF belongs to a trusted proxy (unusual, but possible when
// the entire path is internal infrastructure), we fall through to X-Real-Ip
// and finally to RemoteAddr.
func (c *Ctx) ClientIP() string {
	ipStr, _, err := net.SplitHostPort(c.req.RemoteAddr)
	if err != nil {
		ipStr = c.req.RemoteAddr
	}
	// Parse once; reused for both the trusted-proxy check and as the final
	// fallback return value.  A nil result (malformed RemoteAddr) falls
	// straight through to returning the raw string.
	remoteIP := net.ParseIP(ipStr)

	// Proxy headers are only honoured when the direct peer is a trusted proxy.
	// An untrusted peer cannot influence the result by injecting headers.
	if remoteIP == nil || !c.app.options.isTrustedProxy(remoteIP) {
		if remoteIP != nil {
			return remoteIP.String()
		}
		return ipStr
	}

	// ─── X-Forwarded-For (right-to-left) ─────────────────────────────────────
	if xff := c.req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			parsed := net.ParseIP(candidate)
			if parsed == nil {
				// Malformed entry; keep walking left toward the client.
				continue
			}
			if !c.app.options.isTrustedProxy(parsed) {
				// First IP not belonging to our proxy network → real client.
				return parsed.String()
			}
		}
		// All XFF entries matched trusted proxies; the true origin is beyond
		// our visibility — fall through to X-Real-Ip then RemoteAddr.
	}

	// ─── X-Real-Ip ───────────────────────────────────────────────────────────
	// Single-value header set by upstream (e.g. nginx). Trusted only because
	// we already verified the direct peer is a known proxy (checked above).
	if xri := c.req.Header.Get("X-Real-Ip"); xri != "" {
		if parsed := net.ParseIP(strings.TrimSpace(xri)); parsed != nil {
			return parsed.String()
		}
	}

	return remoteIP.String()
}

// UserAgent returns the User-Agent header.
func (c *Ctx) UserAgent() string {
	return c.req.UserAgent()
}

// IsWebsocket returns true if the request is a WebSocket upgrade request.
func (c *Ctx) IsWebsocket() bool {
	return strings.EqualFold(c.Header("Upgrade"), "websocket")
}
