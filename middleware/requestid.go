package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/astra-go/astra"
)

const requestIDKey = "X-Request-ID"

// RequestID returns a middleware that injects a unique request ID into each request.
// The ID is read from the X-Request-ID header if present, otherwise generated.
// Inspired by go-zero and echo's RequestID middleware.
func RequestID() astra.HandlerFunc {
	return RequestIDWithGenerator(generateID)
}

// RequestIDWithGenerator returns a RequestID middleware with a custom ID generator.
func RequestIDWithGenerator(generator func() string) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		id := c.Header(requestIDKey)
		if id == "" {
			id = generator()
		}
		c.SetHeader(requestIDKey, id)
		c.Set("requestID", id)
		return nil
	}
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(c *astra.Ctx) string {
	return c.GetString("requestID")
}

// idBuf is the pooled buffer for request ID generation.
// Layout: first 16 bytes = random source; last 32 bytes = hex output.
type idBuf struct {
	b [48]byte
}

var idBufPool = sync.Pool{New: func() any { return new(idBuf) }}

// generateID generates a cryptographically random 32-hex-character request ID.
//
// Allocation profile (vs. previous make([]byte,16) + hex.EncodeToString):
//   - Previous: 2 allocs ([]byte + string)
//   - Now:      1 alloc  (string copy — unavoidable since buf is returned to pool)
func generateID() string {
	buf := idBufPool.Get().(*idBuf)
	_, _ = rand.Read(buf.b[:16])
	hex.Encode(buf.b[16:], buf.b[:16])
	id := string(buf.b[16:]) // 1 alloc: must copy before returning buf to pool
	idBufPool.Put(buf)
	return id
}
