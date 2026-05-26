package binding

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

	gojson "github.com/goccy/go-json"
)

// maxBodySize is the default request-body size limit applied by all body binders.
// It guards against request-body DoS attacks.
const maxBodySize = 1 << 20 // 1 MiB

// Binder is the interface for request body binders.
type Binder interface {
	Bind(r *http.Request, obj any) error
}

// Pre-built binder instances.
var (
	JSON = &jsonBinder{}
	XML  = &xmlBinder{}
	Form = &formBinder{}
)

// jsonLRPool pools *io.LimitedReader to avoid the per-request allocation that
// io.LimitReader would otherwise incur on the binding path.
var jsonLRPool = sync.Pool{New: func() any { return new(io.LimitedReader) }}

// jsonReadBufPool pools *bytes.Buffer used to read the request body before
// unmarshalling.  Each buffer is short-lived: obtained, filled, unmarshalled,
// then returned, so pool contention with the response path is negligible.
var jsonReadBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// ─── JSON binder ──────────────────────────────────────────────────────────────

type jsonBinder struct{}

func (b *jsonBinder) Bind(r *http.Request, obj any) error {
	if r.Body == nil {
		return fmt.Errorf("binding: empty request body")
	}
	// Pool a LimitedReader (Plan B): saves 1 alloc vs io.LimitReader.
	lr := jsonLRPool.Get().(*io.LimitedReader)
	lr.R, lr.N = r.Body, maxBodySize

	// Pool a read buffer (Plan B): saves 1 alloc vs io.ReadAll.
	buf := jsonReadBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	_, err := buf.ReadFrom(lr)
	lr.R = nil
	jsonLRPool.Put(lr)

	if err != nil {
		jsonReadBufPool.Put(buf)
		return fmt.Errorf("binding: read body: %w", err)
	}

	// Unmarshal from []byte (Plan A): saves 2 allocs vs json.NewDecoder(*bufio.Reader).
	// goccy/go-json copies all string fields before returning; buf is safe to reuse.
	err = gojson.Unmarshal(buf.Bytes(), obj)
	jsonReadBufPool.Put(buf)
	if err != nil {
		return fmt.Errorf("binding: invalid JSON: %w", err)
	}
	return nil
}

// ─── XML binder ───────────────────────────────────────────────────────────────

type xmlBinder struct{}

func (b *xmlBinder) Bind(r *http.Request, obj any) error {
	if r.Body == nil {
		return fmt.Errorf("binding: empty request body")
	}
	limited := io.LimitReader(r.Body, maxBodySize)
	if err := xml.NewDecoder(limited).Decode(obj); err != nil {
		return fmt.Errorf("binding: invalid XML: %w", err)
	}
	return nil
}

// ─── Form binder ──────────────────────────────────────────────────────────────

type formBinder struct{}

func (b *formBinder) Bind(r *http.Request, obj any) error {
	ct := contentType(r)
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return fmt.Errorf("binding: invalid multipart form")
		}
		return mapValues(obj, r.MultipartForm.Value, "form")
	}
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("binding: invalid form data")
	}
	return mapValues(obj, r.Form, "form")
}

// ─── Multipart helper ─────────────────────────────────────────────────────────

// FormFile retrieves a multipart file upload from the request.
func FormFile(r *http.Request, key string) (*multipart.FileHeader, error) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}
	_, fh, err := r.FormFile(key)
	return fh, err
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func contentType(r *http.Request) string {
	ct := r.Header.Get("Content-Type")
	for i, c := range ct {
		if c == ';' || c == ' ' {
			return ct[:i]
		}
	}
	return ct
}
