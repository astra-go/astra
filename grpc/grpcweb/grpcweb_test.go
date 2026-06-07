package grpcweb

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsGRPCWebRequest(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{contentTypeGRPCWebProto, true},
		{contentTypeGRPCWebText, true},
		{contentTypeGRPCWebJSON, true},
		{"application/grpc-web+proto; charset=utf-8", true},
		{contentTypeGRPCProto, false},
		{"application/json", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsGRPCWebRequest(tt.ct); got != tt.want {
			t.Errorf("IsGRPCWebRequest(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestParseFrame_SingleFrame(t *testing.T) {
	data := []byte{0x00, 0x00, 0x00, 0x00, 0x05, 'h', 'e', 'l', 'l', 'o'}
	frame, err := ParseFrame(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frame.Length != 5 {
		t.Errorf("length = %d, want 5", frame.Length)
	}
	if frame.Compress != 0 {
		t.Errorf("compress = %d, want 0", frame.Compress)
	}
	if string(frame.Data) != "hello" {
		t.Errorf("data = %q, want 'hello'", string(frame.Data))
	}
}

func TestParseFrame_EmptyData(t *testing.T) {
	data := []byte{0x00, 0x00, 0x00, 0x00, 0x00}
	frame, err := ParseFrame(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frame.Length != 0 || len(frame.Data) != 0 {
		t.Errorf("expected empty frame, got length=%d data=%q", frame.Length, frame.Data)
	}
}

func TestParseFrame_Truncated(t *testing.T) {
	data := []byte{0x00, 0x00, 0x00} // too short for header
	_, err := ParseFrame(bytes.NewReader(data))
	if err == nil {
		t.Error("expected error for truncated header")
	}
}

func TestParseFrame_EOF(t *testing.T) {
	_, err := ParseFrame(bytes.NewReader(nil))
	if err != io.ErrUnexpectedEOF {
		t.Errorf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestSerializeFrame(t *testing.T) {
	frame := &Frame{Length: 5, Compress: 0, Data: []byte("hello")}
	buf := SerializeFrame(frame)
	result := buf.Bytes()

	// Verify header
	if len(result) != 10 {
		t.Fatalf("expected 10 bytes, got %d", len(result))
	}
	if result[0] != 0 { // compress
		t.Errorf("byte 0 = %d, want 0", result[0])
	}
	if result[1] != 0 || result[2] != 0 || result[3] != 0 || result[4] != 5 {
		t.Errorf("length header bytes wrong: %v", result[1:5])
	}
	if string(result[5:]) != "hello" {
		t.Errorf("data = %q, want 'hello'", string(result[5:]))
	}
}

func TestParseAndSerialize_RoundTrip(t *testing.T) {
	original := &Frame{Length: 3, Compress: 0, Data: []byte("abc")}
	serialized := SerializeFrame(original)

	parsed, err := ParseFrame(bytes.NewReader(serialized.Bytes()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Length != original.Length {
		t.Errorf("length mismatch: %d vs %d", parsed.Length, original.Length)
	}
	if parsed.Compress != original.Compress {
		t.Errorf("compress mismatch: %d vs %d", parsed.Compress, original.Compress)
	}
	if !bytes.Equal(parsed.Data, original.Data) {
		t.Errorf("data mismatch: %q vs %q", parsed.Data, original.Data)
	}
}

func TestParseFrames_Multiple(t *testing.T) {
	f1 := &Frame{Length: 3, Compress: 0, Data: []byte("abc")}
	f2 := &Frame{Length: 2, Compress: 0, Data: []byte("de")}

	var buf bytes.Buffer
	buf.Write(SerializeFrame(f1).Bytes())
	buf.Write(SerializeFrame(f2).Bytes())

	frames, err := ParseFrames(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if string(frames[0].Data) != "abc" || string(frames[1].Data) != "de" {
		t.Errorf("frames = [%q, %q], want [abc, de]", string(frames[0].Data), string(frames[1].Data))
	}
}

func TestParseFrames_EmptyInput(t *testing.T) {
	frames, err := ParseFrames(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(frames) != 0 {
		t.Errorf("expected 0 frames, got %d", len(frames))
	}
}

func TestSerializeFrames(t *testing.T) {
	frames := []*Frame{
		{Length: 1, Compress: 0, Data: []byte("a")},
		{Length: 2, Compress: 0, Data: []byte("bc")},
	}
	data := SerializeFrames(frames)

	if len(data) != 6+7 { // 5+1 + 5+2
		t.Errorf("expected 13 bytes, got %d", len(data))
	}
}

func TestSerializeAndParse_RoundTrip(t *testing.T) {
	frames := []*Frame{
		{Length: 3, Compress: 0, Data: []byte("abc")},
		{Length: 4, Compress: 0, Data: []byte("wxyz")},
	}
	serialized := SerializeFrames(frames)
	parsed, err := ParseFrames(serialized)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(parsed))
	}
	for i, f := range parsed {
		if !bytes.Equal(f.Data, frames[i].Data) {
			t.Errorf("frame %d data mismatch: %q vs %q", i, f.Data, frames[i].Data)
		}
	}
}

func TestFrameHeaderSize(t *testing.T) {
	if FrameHeaderSize() != 5 {
		t.Errorf("FrameHeaderSize() = %d, want 5", FrameHeaderSize())
	}
}

func TestMetadataFromHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/service.Method", nil)
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	req.Header.Set("X-User-Agent", "test-client")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Host", "localhost")
	req.Header.Set("Content-Length", "100")
	req.Header.Set("TE", "trailers")

	md := MetadataFromHeaders(req)

	// Should include metadata headers
	if md.Get("x-user-agent")[0] != "test-client" {
		t.Errorf("missing x-user-agent metadata")
	}
	if md.Get("authorization")[0] != "Bearer token123" {
		t.Errorf("missing authorization metadata")
	}
	if md.Get("x-custom-header")[0] != "custom-value" {
		t.Errorf("missing x-custom-header metadata")
	}

	// Should NOT include transport headers
	if len(md.Get("content-type")) > 0 {
		t.Error("should not include content-type in metadata")
	}
	if len(md.Get("host")) > 0 {
		t.Error("should not include host in metadata")
	}
	if len(md.Get("content-length")) > 0 {
		t.Error("should not include content-length in metadata")
	}
}

func TestMetadataFromHeaders_EmptyRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	md := MetadataFromHeaders(req)
	if len(md) != 0 {
		t.Errorf("expected empty metadata for headerless request, got %v", md)
	}
}

func TestWrapper_ServeHTTP_PassThrough(t *testing.T) {
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	wrapper := &Wrapper{
		opts: DefaultOptions(),
		handler: next,
	}

	// Non-gRPC-Web request should pass through
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler should have been called for non-gRPC-Web request")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestWrapper_ServeHTTP_PassThrough_PlainHTTP(t *testing.T) {
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	wrapper := &Wrapper{
		opts:    DefaultOptions(),
		handler: next,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapper.ServeHTTP(rec, req)

	if !called {
		t.Error("next handler should have been called for plain HTTP request")
	}
}

func TestWrapper_ServeHTTP_CORS_Preflight(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should NOT be called for OPTIONS preflight")
	})

	wrapper := &Wrapper{
		opts:    DefaultOptions(),
		handler: next,
	}

	req := httptest.NewRequest(http.MethodOptions, "/service.Method", nil)
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	ao := rec.Header().Get("Access-Control-Allow-Origin")
	if ao != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", ao)
	}
}

func TestWrapper_ServeHTTP_CORS_WithAllowedOrigins(t *testing.T) {
	wrapper := &Wrapper{
		opts: Options{
			AllowedOrigins:   []string{"https://allowed.com", "https://also.com"},
			AllowAllOrigins:  false,
			TrailersKey:      "grpc-web-",
			MaxRequestSize:   4 * 1024 * 1024,
		},
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	// Allowed origin
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	req.Header.Set("Origin", "https://allowed.com")
	rec := httptest.NewRecorder()
	wrapper.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://allowed.com" {
		t.Errorf("expected 'https://allowed.com', got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	// Disallowed origin
	req.Header.Set("Origin", "https://evil.com")
	rec = httptest.NewRecorder()
	wrapper.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected empty origin for disallowed, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestWrapper_ServeHTTP_GRPCWeb_Basic(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should NOT be called for gRPC-Web request")
	})

	wrapper := &Wrapper{
		opts:    DefaultOptions(),
		handler: next,
	}

	// Build a valid gRPC-Web unary request
	reqFrame := &Frame{
		Compress: FrameNoCompress,
		Data:     []byte("test-payload"),
	}
	reqBody := SerializeFrame(reqFrame)

	req := httptest.NewRequest(http.MethodPost, "/service.Method", bytes.NewReader(reqBody.Bytes()))
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	// Should return 200 with gRPC-Web content type
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != contentTypeGRPCWebProto {
		t.Errorf("Content-Type = %q, want %q", ct, contentTypeGRPCWebProto)
	}

	// Should have gRPC status headers
	if rec.Header().Get("Grpc-Status") != "0" {
		t.Errorf("Grpc-Status = %q, want 0", rec.Header().Get("Grpc-Status"))
	}

	// Response body should contain at least 2 frames (data + trailer)
	respFrames, err := ParseFrames(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("failed to parse response frames: %v", err)
	}
	if len(respFrames) < 2 {
		t.Errorf("expected at least 2 response frames, got %d", len(respFrames))
	}
}

func TestWrapper_ServeHTTP_GRPCWeb_TextEncoding(t *testing.T) {
	wrapper := &Wrapper{
		opts:    DefaultOptions(),
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	reqFrame := &Frame{Compress: FrameNoCompress, Data: []byte("text-test")}
	reqBody := SerializeFrame(reqFrame)

	req := httptest.NewRequest(http.MethodPost, "/service.Method", bytes.NewReader(reqBody.Bytes()))
	req.Header.Set("Content-Type", contentTypeGRPCWebText)
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	// Text encoding: response body should be base64
	ct := rec.Header().Get("Content-Type")
	if ct != contentTypeGRPCWebText {
		t.Errorf("Content-Type = %q, want %q", ct, contentTypeGRPCWebText)
	}

	// Verify it's valid base64
	_, err := base64.StdEncoding.DecodeString(rec.Body.String())
	if err != nil {
		t.Errorf("response body is not valid base64: %v", err)
	}
}

func TestWrapper_ServeHTTP_GRPCWeb_EmptyBody(t *testing.T) {
	wrapper := &Wrapper{
		opts:    DefaultOptions(),
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	req := httptest.NewRequest(http.MethodPost, "/service.Method", bytes.NewReader(nil))
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	// Empty body → 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for empty body", rec.Code)
	}
}

func TestWrapper_ServeHTTP_GRPCWeb_TooLarge(t *testing.T) {
	wrapper := &Wrapper{
		opts:    Options{
			AllowAllOrigins: true,
			MaxRequestSize:  10, // 10 bytes max
			TrailersKey:      "grpc-web-",
		},
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	req := httptest.NewRequest(http.MethodPost, "/service.Method", bytes.NewReader(make([]byte, 100)))
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	req.Header.Set("Origin", "https://example.com")
	req.ContentLength = 100
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413 for too large body", rec.Code)
	}
}

func TestWrapper_ServeHTTP_GRPCWeb_CORSHeaders(t *testing.T) {
	wrapper := &Wrapper{
		opts: Options{
			AllowAllOrigins:     true,
			AllowCustomMetadata: true,
			MaxRequestSize:      4 * 1024 * 1024,
			TrailersKey:         "grpc-web-",
		},
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	}

	reqFrame := &Frame{Compress: FrameNoCompress, Data: []byte("cors-test")}
	reqBody := SerializeFrame(reqFrame)

	req := httptest.NewRequest(http.MethodPost, "/service.Method", bytes.NewReader(reqBody.Bytes()))
	req.Header.Set("Content-Type", contentTypeGRPCWebProto)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	wrapper.ServeHTTP(rec, req)

	// Verify CORS headers
	ao := rec.Header().Get("Access-Control-Allow-Origin")
	if ao != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", ao)
	}

	am := rec.Header().Get("Access-Control-Allow-Methods")
	if am != "POST, GET, OPTIONS" {
		t.Errorf("Access-Control-Allow-Methods = %q", am)
	}

	ah := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(ah, "Content-Type") || !strings.Contains(ah, "X-Grpc-Web") {
		t.Errorf("Access-Control-Allow-Headers = %q", ah)
	}

	// Verify custom metadata is allowed
	ahvs := rec.Header().Values("Access-Control-Allow-Headers")
	found := false
	for _, v := range ahvs {
		if strings.Contains(v, "X-Custom-Metadata-*") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected X-Custom-Metadata-* in Allow-Headers, got %v", ahvs)
	}

	// Trailer exposure
	ae := rec.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(ae, "X-Grpc-Web") || !strings.Contains(ae, "grpc-web-") {
		t.Errorf("Access-Control-Expose-Headers = %q", ae)
	}
}

func TestEncodeGRPCWebTrailers(t *testing.T) {
	// OK status, no message
	data := encodeGRPCWebTrailers(0, "")
	if len(data) != 5 {
		t.Errorf("expected 5 bytes, got %d", len(data))
	}
	if data[0] != 0 {
		t.Errorf("status byte = %d, want 0", data[0])
	}

	// Non-OK status with message
	msg := "not found"
	data = encodeGRPCWebTrailers(5, msg)
	if len(data) != 5+len(msg) {
		t.Errorf("expected %d bytes, got %d", 5+len(msg), len(data))
	}
	if data[0] != 5 {
		t.Errorf("status byte = %d, want 5", data[0])
	}
	// Verify message length encoding
	msgLen := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
	if int(msgLen) != len(msg) {
		t.Errorf("message length = %d, want %d", msgLen, len(msg))
	}
}

func TestAllowedContentTypes(t *testing.T) {
	types := AllowedContentTypes()
	if len(types) != 3 {
		t.Errorf("expected 3 content types, got %d", len(types))
	}
}

func TestIsGRPCWebContentType_Alias(t *testing.T) {
	// IsGRPCWebContentType is a public alias for IsGRPCWebRequest
	if !IsGRPCWebContentType(contentTypeGRPCWebProto) {
		t.Error("proto content type should be recognized")
	}
	if IsGRPCWebContentType("text/html") {
		t.Error("html should not be recognized")
	}
}

func TestDefaultOptions(t *testing.T) {
	o := DefaultOptions()
	if !o.AllowAllOrigins {
		t.Error("default should allow all origins")
	}
	if o.MaxRequestSize != 4*1024*1024 {
		t.Errorf("max request size = %d, want 4MB", o.MaxRequestSize)
	}
	if o.TrailersKey != "grpc-web-" {
		t.Errorf("trailers key = %q, want 'grpc-web-'", o.TrailersKey)
	}
}

func TestFunctionalOptions(t *testing.T) {
	o := DefaultOptions()
	WithAllowedOrigins([]string{"https://a.com"})(&o)
	WithMaxRequestSize(1024)(&o)
	WithTrailersKey("custom-")(&o)
	WithAllowCustomMetadata()(&o)

	if o.AllowAllOrigins {
		t.Error("should not allow all origins when WithAllowedOrigins is used")
	}
	if o.MaxRequestSize != 1024 {
		t.Errorf("max request size = %d, want 1024", o.MaxRequestSize)
	}
	if o.TrailersKey != "custom-" {
		t.Errorf("trailers key = %q, want 'custom-'", o.TrailersKey)
	}
	if !o.AllowCustomMetadata {
		t.Error("custom metadata should be allowed")
	}
}

func TestResponseRecorder(t *testing.T) {
	var buf bytes.Buffer
	rec := &responseRecorder{
		header: make(http.Header),
		body:   &buf,
		code:   0,
	}

	rec.Header().Set("X-Custom", "value")
	if rec.Header().Get("X-Custom") != "value" {
		t.Error("header not set")
	}

	n, err := rec.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("Write failed: n=%d err=%v", n, err)
	}

	rec.WriteHeader(http.StatusCreated)
	if rec.code != http.StatusCreated {
		t.Errorf("code = %d, want 201", rec.code)
	}
}
