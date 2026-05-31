package stream

import (
	"io"
	"net/http"
	"sync"

	"github.com/astra-go/astra"
	gorilla "github.com/gorilla/websocket"
)

// clientCtx implements astra.ClientStream over a WebSocket connection.
// The client sends a sequence of DATA frames ending with an END frame;
// the server reads them and replies exactly once via SendAndClose.
type clientCtx struct {
	*astra.Ctx
	conn      *gorilla.Conn
	codec     astra.Serializer
	done      chan struct{}
	once      sync.Once
	mu        sync.Mutex // guards Send-side writes
	ended     bool       // true after SendAndClose has been called
	recvLimit *msgRateLimiter // nil = no rate limit
}

func newClientCtx(c *astra.Ctx, conn *gorilla.Conn, o Options) *clientCtx {
	var lim *msgRateLimiter
	if o.RateLimit != nil {
		lim = newMsgRateLimiter(o.RateLimit.Rate, o.RateLimit.Burst)
	}
	return &clientCtx{
		Ctx:       c,
		conn:      conn,
		codec:     o.Codec,
		done:      make(chan struct{}),
		recvLimit: lim,
	}
}

// Recv reads the next message from the client into v.
// Returns io.EOF when the client has finished sending (END frame or normal WS close).
// Returns ErrRateLimited if the per-stream receive rate limit is exceeded.
func (s *clientCtx) Recv(v any) error {
	if s.recvLimit != nil && !s.recvLimit.allow() {
		return ErrRateLimited
	}
	_, msg, err := s.conn.ReadMessage()
	if err != nil {
		s.close()
		if gorilla.IsCloseError(err, gorilla.CloseNormalClosure, gorilla.CloseGoingAway) {
			return io.EOF
		}
		return err
	}

	f, err := decodeFrame(msg)
	if err != nil {
		return err
	}
	switch f.typ {
	case frameTypeEnd:
		s.close()
		return io.EOF
	case frameTypePing:
		s.mu.Lock()
		_ = s.conn.WriteMessage(gorilla.BinaryMessage, encodeFrame(nil, frameTypePong, nil))
		s.mu.Unlock()
		return s.Recv(v)
	case frameTypeError:
		s.close()
		return &StreamError{Message: string(f.payload)}
	}
	return s.codec.Unmarshal(f.payload, v)
}

// Next overrides Ctx.Next() to match contract.Context signature (no return value).
func (s *clientCtx) Next() {
	_ = s.Ctx.Next()
}

// SendAndClose sends a single response to the client and closes the stream.
// Must be called exactly once after all Recv calls are complete.
func (s *clientCtx) SendAndClose(v any) error {
	if s.ended {
		return &StreamError{Message: "SendAndClose called more than once"}
	}
	s.ended = true

	payload, err := s.codec.Marshal(v)
	if err != nil {
		return err
	}

	buf := frameBufPool.Get().(*[]byte)
	*buf = (*buf)[:0]
	*buf = encodeFrame(*buf, frameTypeData, payload)
	*buf = encodeFrame(*buf, frameTypeEnd, nil)

	s.mu.Lock()
	err = s.conn.WriteMessage(gorilla.BinaryMessage, *buf)
	s.mu.Unlock()

	frameBufPool.Put(buf)
	s.close()
	return err
}

func (s *clientCtx) Done() <-chan struct{} { return s.done }

func (s *clientCtx) close() {
	s.once.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}

// ClientStreamHandler wraps a ClientStreamHandler as a standard astra.HandlerFunc
// that can be registered with app.GET.
//
// The handler upgrades the HTTP connection to WebSocket. The client streams
// DATA frames; the server reads them via Recv, then calls SendAndClose once.
//
// Requires standard net/http mode. Returns HTTP 500 when called under
// RunReactor (netengine does not support connection hijacking).
//
// Example:
//
//	app.GET("/upload", stream.ClientStreamHandler(func(s astra.ClientStream) error {
//	    var total int
//	    for {
//	        var chunk Chunk
//	        err := s.Recv(&chunk)
//	        if errors.Is(err, io.EOF) { break }
//	        if err != nil { return err }
//	        total += len(chunk.Data)
//	    }
//	    return s.SendAndClose(Result{Total: total})
//	}))
func ClientStreamHandler(fn astra.ClientStreamHandler, opts ...Option) astra.HandlerFunc {
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

		if _, ok := c.Writer().(http.Hijacker); !ok {
			return astra.NewHTTPError(http.StatusInternalServerError,
				"stream: ClientStream requires net/http mode; RunReactor does not support connection hijacking")
		}

		conn, err := wsUpgrader.Upgrade(c.Writer(), c.Request(), nil)
		if err != nil {
			return astra.NewHTTPError(http.StatusBadRequest, "stream: WebSocket upgrade failed: "+err.Error())
		}
		conn.SetReadLimit(o.ReadLimit)

		s := newClientCtx(c, conn, o)
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
