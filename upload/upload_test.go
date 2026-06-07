package upload_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/upload"
)

// buildMultipart builds a multipart/form-data body with fields and files.
func buildMultipart(t *testing.T, fields map[string]string, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for k, content := range files {
		fw, err := w.CreateFormFile(k, k+".txt")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(content)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

// testApp creates an Astra app + httptest.Server.
func testApp(t *testing.T, handler astra.HandlerFunc, middleware ...astra.HandlerFunc) *httptest.Server {
	t.Helper()
	app := astra.New()
	for _, mw := range middleware {
		app.Use(mw)
	}
	app.POST("/upload", handler)
	return httptest.NewServer(app)
}

// ─── StreamLimit ─────────────────────────────────────────────────────────────

func TestStreamLimit_AllowsSmallBody(t *testing.T) {
	srv := testApp(t,
		func(c *astra.Ctx) error { return c.String(200, "ok") },
		upload.StreamLimit(1024),
	)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/upload", "text/plain", bytes.NewBufferString("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestStreamLimit_RejectsLargeBody(t *testing.T) {
	srv := testApp(t,
		func(c *astra.Ctx) error {
			// Read the body to trigger MaxBytesReader.
			_, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return astra.NewHTTPError(http.StatusRequestEntityTooLarge, "body too large")
			}
			return c.String(200, "ok")
		},
		upload.StreamLimit(100),
	)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/upload", "text/plain", bytes.NewBuffer(make([]byte, 200)))
	if err != nil {
		return // connection closed is also valid
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("expected non-200 for oversized body")
	}
}

// ─── SaveFile ────────────────────────────────────────────────────────────────

func TestSaveFile(t *testing.T) {
	tmpDir := t.TempDir()

	srv := testApp(t, func(c *astra.Ctx) error {
		path, header, err := upload.SaveFile(c, "file", upload.SaveFileConfig{
			MaxSize: 1 << 20,
			DestDir: tmpDir,
		})
		if err != nil {
			return err
		}
		if header == nil {
			return astra.NewHTTPError(500, "nil header")
		}
		if _, err := os.Stat(path); err != nil {
			return astra.NewHTTPError(500, "file not found: "+err.Error())
		}
		return c.JSON(200, map[string]string{"path": path})
	})
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{"file": []byte("hello, world!")})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestSaveFile_TooLarge(t *testing.T) {
	tmpDir := t.TempDir()

	srv := testApp(t, func(c *astra.Ctx) error {
		_, _, err := upload.SaveFile(c, "file", upload.SaveFileConfig{
			MaxSize: 5,
			DestDir: tmpDir,
		})
		if err == nil {
			return astra.NewHTTPError(500, "expected error")
		}
		return astra.NewHTTPError(400, err.Error())
	})
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{"file": []byte("this is way more than 5 bytes!")})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSaveFile_ExtensionFilter(t *testing.T) {
	tmpDir := t.TempDir()

	srv := testApp(t, func(c *astra.Ctx) error {
		_, _, err := upload.SaveFile(c, "file", upload.SaveFileConfig{
			MaxSize:     1 << 20,
			DestDir:     tmpDir,
			AllowedExts: []string{".png", ".jpg"},
		})
		if err == nil {
			return astra.NewHTTPError(500, "expected error")
		}
		return astra.NewHTTPError(400, err.Error())
	})
	defer srv.Close()

	// File has .txt extension, not in allowed list.
	body, ct := buildMultipart(t, nil, map[string][]byte{"file": []byte("data")})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSaveFile_CleanupOnError(t *testing.T) {
	tmpDir := t.TempDir()

	srv := testApp(t, func(c *astra.Ctx) error {
		upload.SaveFile(c, "file", upload.SaveFileConfig{
			MaxSize: 5,
			DestDir: tmpDir,
		})
		return c.String(400, "fail")
	})
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{"file": []byte("way too big!")})
	resp, _ := http.Post(srv.URL+"/upload", ct, body)
	if resp != nil {
		resp.Body.Close()
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 leftover files, found %d", len(entries))
	}
}

func TestSaveFile_DestDirCreated(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "deep")

	srv := testApp(t, func(c *astra.Ctx) error {
		path, _, err := upload.SaveFile(c, "file", upload.SaveFileConfig{
			MaxSize: 1 << 20,
			DestDir: nestedDir,
		})
		if err != nil {
			return err
		}
		return c.JSON(200, map[string]string{"path": path})
	})
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{"file": []byte("data")})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if _, err := os.Stat(nestedDir); err != nil {
		t.Errorf("nested dir should exist: %v", err)
	}
}

// ─── SaveFileTo ──────────────────────────────────────────────────────────────

func TestSaveFileTo(t *testing.T) {
	var buf bytes.Buffer

	srv := testApp(t, func(c *astra.Ctx) error {
		header, err := upload.SaveFileTo(c, "file", &buf, upload.SaveFileConfig{
			MaxSize: 1 << 20,
		})
		if err != nil {
			return err
		}
		if header == nil {
			return astra.NewHTTPError(500, "nil header")
		}
		return c.String(200, "ok")
	})
	defer srv.Close()

	content := []byte("stream me")
	body, ct := buildMultipart(t, nil, map[string][]byte{"file": content})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !bytes.Contains(buf.Bytes(), content) {
		t.Error("writer should contain uploaded content")
	}
}

// ─── UploadLimiter ───────────────────────────────────────────────────────────

func TestUploadLimiter_WithinLimits(t *testing.T) {
	srv := testApp(t,
		func(c *astra.Ctx) error { return c.String(200, "ok") },
		upload.UploadLimiter(upload.UploadLimiterConfig{
			MaxBodySize:  1 << 20,
			MaxFileSize:  512,
			MaxFiles:     2,
			MaxFieldSize: 100,
		}),
	)
	defer srv.Close()

	body, ct := buildMultipart(t,
		map[string]string{"note": "hi"},
		map[string][]byte{"file1": []byte("small")},
	)
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUploadLimiter_FileTooLarge(t *testing.T) {
	srv := testApp(t,
		func(c *astra.Ctx) error { return c.String(200, "ok") },
		upload.UploadLimiter(upload.UploadLimiterConfig{
			MaxFileSize: 10,
		}),
	)
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{"file": make([]byte, 100)})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("expected non-200 for oversized file")
	}
}

func TestUploadLimiter_TooManyFiles(t *testing.T) {
	srv := testApp(t,
		func(c *astra.Ctx) error { return c.String(200, "ok") },
		upload.UploadLimiter(upload.UploadLimiterConfig{
			MaxFiles:    1,
			MaxFileSize: 1 << 20,
		}),
	)
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{
		"file1": []byte("a"),
		"file2": []byte("b"),
	})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("expected non-200 for too many files")
	}
}

func TestUploadLimiter_ExtensionFilter(t *testing.T) {
	srv := testApp(t,
		func(c *astra.Ctx) error { return c.String(200, "ok") },
		upload.UploadLimiter(upload.UploadLimiterConfig{
			MaxFileSize: 1 << 20,
			AllowedExts: []string{".png"},
		}),
	)
	defer srv.Close()

	body, ct := buildMultipart(t, nil, map[string][]byte{"file": []byte("data")})
	resp, err := http.Post(srv.URL+"/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
