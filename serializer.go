package astra

import (
	"bytes"
	"io"
	"sync"

	gojson "github.com/goccy/go-json"
)

// Serializer handles JSON marshalling and unmarshalling.
// Swap the default encoding/json with faster alternatives (e.g. sonic, jsoniter):
//
//	import "github.com/bytedance/sonic"
//	app := astra.New(astra.WithSerializer(sonic.ConfigStd))
type Serializer interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// bufEncoder is an optional interface that a Serializer can implement to write
// JSON directly into an existing *bytes.Buffer, avoiding the intermediate []byte
// that Marshal allocates and that must then be copied into the response buffer.
// context_response.JSON checks for this interface to use the zero-copy path.
type bufEncoder interface {
	EncodeInto(buf *bytes.Buffer, v any) error
}

// streamEncoder is an optional interface that a Serializer can implement to
// write JSON directly into any io.Writer without an intermediate buffer.
// context_response.JSONStream checks for this interface to skip the pooled
// *bytes.Buffer entirely, eliminating the copy for large list responses.
type streamEncoder interface {
	EncodeStream(w io.Writer, v any) error
}

// goJsonSerializer is the default Serializer backed by github.com/goccy/go-json.
// It is a drop-in replacement for encoding/json with fewer allocations on the
// marshal path (no reflect-based interface boxing for common scalar types).
// It also implements bufEncoder to write directly into a pooled *bytes.Buffer,
// saving 1 alloc (the intermediate []byte) on the JSON response path.
type goJsonSerializer struct{}

func (goJsonSerializer) Marshal(v any) ([]byte, error)      { return gojson.Marshal(v) }
func (goJsonSerializer) Unmarshal(data []byte, v any) error { return gojson.Unmarshal(data, v) }

// reuseWriter is a thin io.Writer shim whose target can be redirected before
// each Encode call, letting us pool a single *gojson.Encoder across requests.
// The target is typed as io.Writer so the same pool serves both EncodeInto
// (*bytes.Buffer) and EncodeStream (http.ResponseWriter).
type reuseWriter struct{ w io.Writer }

func (rw *reuseWriter) Write(p []byte) (int, error) { return rw.w.Write(p) }

// reusableEncoder pairs a pooled *gojson.Encoder with its *reuseWriter so both
// can be stored and retrieved together without extra interface allocations.
type reusableEncoder struct {
	rw  *reuseWriter
	enc *gojson.Encoder
}

// goJsonEncPool holds warm *reusableEncoder values.  After pool warm-up, each
// JSON response incurs 0 allocs for the Encoder itself.
var goJsonEncPool = sync.Pool{New: func() any {
	rw := &reuseWriter{}
	enc := gojson.NewEncoder(rw)
	enc.SetEscapeHTML(false)
	return &reusableEncoder{rw: rw, enc: enc}
}}

// EncodeInto implements bufEncoder.  It writes the JSON encoding of v directly
// into buf, trimming the trailing '\n' that gojson.Encoder.Encode appends.
func (goJsonSerializer) EncodeInto(buf *bytes.Buffer, v any) error {
	re := goJsonEncPool.Get().(*reusableEncoder)
	re.rw.w = buf
	err := re.enc.Encode(v)
	// Trim the trailing '\n' appended by Encoder.Encode so JSON responses are
	// byte-for-byte identical to what Marshal would have produced.
	if err == nil {
		if b := buf.Bytes(); len(b) > 0 && b[len(b)-1] == '\n' {
			buf.Truncate(buf.Len() - 1)
		}
	}
	re.rw.w = nil
	goJsonEncPool.Put(re)
	return err
}

// EncodeStream implements streamEncoder.  It writes the JSON encoding of v
// directly into w without an intermediate buffer.  The trailing '\n' appended
// by Encoder.Encode is left as-is; it is valid JSON whitespace and harmless for
// streaming responses where Content-Length is not set.
func (goJsonSerializer) EncodeStream(w io.Writer, v any) error {
	re := goJsonEncPool.Get().(*reusableEncoder)
	re.rw.w = w
	err := re.enc.Encode(v)
	re.rw.w = nil
	goJsonEncPool.Put(re)
	return err
}

var defaultSerializer Serializer = goJsonSerializer{}
