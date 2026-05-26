package stream

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
	gorilla "github.com/gorilla/websocket"
)

var wsUpgrader = gorilla.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// bidiCtx implements astra.BidiStream over a WebSocket connection.
type bidiCtx struct {
	*astra.Ctx
	conn      *gorilla.Conn
	codec     astra.Serializer
	done      chan struct{}
	once      sync.Once
	mu        sync.Mutex // guards concurrent Send calls
	sendLimit *msgRateLimiter // nil = no rate limit
	recvLimit *msgRateLimiter // nil = no rate limit
}

func newBidiCtx(c *astra.Ctx, conn *gorilla.Conn, o Options) *bidiCtx {
	var sendLim, recvLim *msgRateLimiter
	if o.RateLimit != nil {
		sendLim = newMsgRateLimiter(o.RateLimit.Rate, o.RateLimit.Burst)
		recvLim = newMsgRateLimiter(o.RateLimit.Rate, o.RateLimit.Burst)
	}
	return &bidiCtx{
		Ctx:       c,
		conn:      conn,
		codec:     o.Codec,
		done:      make(chan struct{}),
		sendLimit: sendLim,
		recvLimit: recvLim,
	}
}

func (s *bidiCtx) Send(v any) error {
	if s.sendLimit != nil && !s.sendLimit.allow() {
		return ErrRateLimited
	}
	payload, err := s.codec.Marshal(v)
	if err != nil {
		return err
	}

	buf := frameBufPool.Get().(*[]byte)
	*buf = (*buf)[:0]
	*buf = encodeFrame(*buf, frameTypeData, payload)

	s.mu.Lock()
	err = s.conn.WriteMessage(gorilla.BinaryMessage, *buf)
	s.mu.Unlock()

	frameBufPool.Put(buf)
	return err
}

func (s *bidiCtx) Recv(v any) error {
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

func (s *bidiCtx) Done() <-chan struct{} { return s.done }

func (s *bidiCtx) close() {
	s.once.Do(func() {
		close(s.done)
		buf := frameBufPool.Get().(*[]byte)
		*buf = (*buf)[:0]
		*buf = encodeFrame(*buf, frameTypeEnd, nil)
		s.mu.Lock()
		_ = s.conn.WriteMessage(gorilla.BinaryMessage, *buf)
		s.mu.Unlock()
		frameBufPool.Put(buf)
		_ = s.conn.Close()
	})
}

// BidiHandler wraps a BidiStreamHandler as a standard astra.HandlerFunc that
// can be registered with app.GET.
//
// The handler upgrades the HTTP connection to WebSocket and then calls fn.
// Messages are framed with the stream binary protocol (5-byte header).
//
// Requires standard net/http mode. Returns HTTP 500 when called under
// RunReactor (netengine does not support connection hijacking).
//
// Example:
//
//	app.GET("/chat", stream.BidiHandler(func(s astra.BidiStream) error {
//	    for {
//	        var msg Message
//	        if err := s.Recv(&msg); errors.Is(err, io.EOF) { return nil }
//	        s.Send(Reply{Text: "echo: " + msg.Text})
//	    }
//	}))
func BidiHandler(fn astra.BidiStreamHandler, opts ...Option) astra.HandlerFunc {
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
				"stream: BidiStream requires net/http mode; RunReactor does not support connection hijacking")
		}

		conn, err := wsUpgrader.Upgrade(c.Writer(), c.Request(), nil)
		if err != nil {
			return astra.NewHTTPError(http.StatusBadRequest, "stream: WebSocket upgrade failed: "+err.Error())
		}
		conn.SetReadLimit(o.ReadLimit)

		s := newBidiCtx(c, conn, o)
		defer s.close()

		if o.PingInterval > 0 {
			go bidiPingLoop(s, o.PingInterval)
		}

		// Close done channel when the request context is cancelled (client disconnected).
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

func bidiPingLoop(s *bidiCtx, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	buf := encodeFrame(nil, frameTypePing, nil)
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.mu.Lock()
			err := s.conn.WriteMessage(gorilla.BinaryMessage, buf)
			s.mu.Unlock()
			if err != nil {
				s.close()
				return
			}
		}
	}
}

// StreamError is returned by Recv when the remote side sent an error frame.
type StreamError struct {
	Message string
}

func (e *StreamError) Error() string { return "stream error: " + e.Message }
