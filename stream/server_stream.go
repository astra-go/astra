package stream

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/astra-go/astra"
)

// sseCtx implements astra.ServerStream over HTTP text/event-stream (SSE).
type sseCtx struct {
	*astra.Ctx
	flusher http.Flusher
	codec   astra.Serializer
	done    chan struct{}
	once    sync.Once
	limiter *msgRateLimiter // nil = no rate limit
}

func newSSECtx(c *astra.Ctx, flusher http.Flusher, o Options) *sseCtx {
	var lim *msgRateLimiter
	if o.RateLimit != nil {
		lim = newMsgRateLimiter(o.RateLimit.Rate, o.RateLimit.Burst)
	}
	return &sseCtx{
		Ctx:     c,
		flusher: flusher,
		codec:   o.Codec,
		done:    make(chan struct{}),
		limiter: lim,
	}
}

// Send serializes v as JSON and writes it as an SSE data line, then flushes.
// Returns ErrRateLimited if the per-stream rate limit is exceeded.
func (s *sseCtx) Send(v any) error {
	if s.limiter != nil && !s.limiter.allow() {
		return ErrRateLimited
	}
	payload, err := s.codec.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.Writer(), "data: %s\n\n", payload); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *sseCtx) Done() <-chan struct{} { return s.done }

func (s *sseCtx) close() {
	s.once.Do(func() { close(s.done) })
}

// ServerStreamHandler wraps a ServerStreamHandler as a standard astra.HandlerFunc
// that can be registered with app.GET.
//
// Each call opens an HTTP text/event-stream response and calls fn. The handler
// should call Send repeatedly to push messages to the client.
//
// Requires standard net/http mode. Returns HTTP 500 when called under
// RunReactor (netengine buffers the entire response and does not support Flush).
//
// Example:
//
//	app.GET("/progress", stream.ServerStreamHandler(func(s astra.ServerStream) error {
//	    for i := 0; i <= 100; i += 10 {
//	        if err := s.Send(Progress{Pct: i}); err != nil { return err }
//	        time.Sleep(200 * time.Millisecond)
//	    }
//	    return nil
//	}))
func ServerStreamHandler(fn astra.ServerStreamHandler, opts ...Option) astra.HandlerFunc {
	o := buildOptions(opts)
	var cl *connLimiter
	if o.MaxConns > 0 {
		cl = newConnLimiter(o.MaxConns)
	}
	return func(c *astra.Ctx) error {
		if cl != nil {
			ok, release := cl.acquire(c.Request().RemoteAddr)
			if !ok {
				return astra.NewHTTPError(http.StatusTooManyRequests,
					"stream: too many concurrent connections from this client")
			}
			defer release()
		}

		flusher, ok := c.Writer().(http.Flusher)
		if !ok {
			return astra.NewHTTPError(http.StatusInternalServerError,
				"stream: ServerStream requires net/http mode; RunReactor does not support streaming flush")
		}

		h := c.Writer().Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")
		c.Writer().WriteHeader(http.StatusOK)

		s := newSSECtx(c, flusher, o)
		defer s.close()

		go func() {
			select {
			case <-c.Request().Context().Done():
				s.close()
			case <-s.done:
			}
		}()

		return fn(s)
	}
}
