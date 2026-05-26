package stream

import (
	"encoding/json"
	"time"

	"github.com/astra-go/astra"
)

const (
	defaultPingInterval = 30 * time.Second
	defaultReadLimit    = 4 << 20 // 4 MB
)

// StreamRateLimit configures per-message rate limiting within an active stream.
type StreamRateLimit struct {
	// Rate is the number of messages allowed per second.
	Rate float64
	// Burst is the maximum burst size (initial token count).
	Burst int
}

// Options configures the behaviour of a stream handler.
type Options struct {
	// Codec is the serializer used for message payloads.
	// Defaults to standard encoding/json.
	Codec astra.Serializer

	// PingInterval is how often WebSocket PING frames are sent to keep the
	// connection alive. Set to 0 to disable pings.
	// Defaults to 30s.
	PingInterval time.Duration

	// ReadLimit caps the maximum size of a single incoming WebSocket message.
	// Defaults to 4 MB.
	ReadLimit int64

	// RateLimit enables per-message rate limiting within an active stream using
	// a token-bucket algorithm. nil = no limit.
	//
	// For ServerStream this limits Send calls (server→client message rate).
	// For ClientStream this limits Recv calls (client→server message rate).
	// For BidiStream this limits both Send and Recv independently.
	//
	// When exceeded, Send/Recv returns ErrRateLimited.
	RateLimit *StreamRateLimit

	// MaxConns is the maximum number of concurrent active streams from a single
	// client IP. Requests beyond the limit receive HTTP 429. 0 = unlimited.
	MaxConns int
}

// Option is a functional option for stream handlers.
type Option func(*Options)

// WithCodec overrides the default JSON codec with the supplied serializer.
// This lets callers plug in faster libraries such as sonic or msgpack.
func WithCodec(s astra.Serializer) Option {
	return func(o *Options) { o.Codec = s }
}

// WithPingInterval sets the WebSocket ping cadence. Pass 0 to disable pings.
func WithPingInterval(d time.Duration) Option {
	return func(o *Options) { o.PingInterval = d }
}

// WithReadLimit sets the maximum allowed incoming message size in bytes.
func WithReadLimit(n int64) Option {
	return func(o *Options) { o.ReadLimit = n }
}

// WithRateLimit enables per-message rate limiting within the active stream.
// rate is messages per second; burst is the initial (and maximum) token count.
//
// When Send or Recv exceeds the limit, ErrRateLimited is returned.
func WithRateLimit(rate float64, burst int) Option {
	return func(o *Options) { o.RateLimit = &StreamRateLimit{Rate: rate, Burst: burst} }
}

// WithMaxConns limits the number of concurrent active streams from a single
// client IP to n. Connections beyond the limit receive HTTP 429 before the
// WebSocket upgrade or SSE headers are written. Pass 0 to remove any limit.
func WithMaxConns(n int) Option {
	return func(o *Options) { o.MaxConns = n }
}

func buildOptions(opts []Option) Options {
	o := Options{
		Codec:        jsonCodec{},
		PingInterval: defaultPingInterval,
		ReadLimit:    defaultReadLimit,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// jsonCodec wraps encoding/json to satisfy astra.Serializer without importing
// the internal gojson pool from the core module.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)          { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error     { return json.Unmarshal(data, v) }
