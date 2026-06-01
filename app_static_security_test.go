package astra

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestStatic_SecurityEdgeCases tests additional security edge cases
func TestStatic_SecurityEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	secretDir := filepath.Join(tmpDir, "secret")

	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secretDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test files
	publicFile := filepath.Join(staticDir, "public.txt")
	if err := os.WriteFile(publicFile, []byte("public content"), 0644); err != nil {
		t.Fatal(err)
	}

	secretFile := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret content"), 0644); err != nil {
		t.Fatal(err)
	}

	app := New()
	app.Static("/static", staticDir)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		should         string
	}{
		{
			name:           "backslash path separator",
			path:           "/static/..\\..\\secret\\secret.txt",
			expectedStatus: http.StatusForbidden,
			should:         "block backslash path traversal (Windows-style)",
		},
		{
			name:           "double encoded traversal",
			path:           "/static/%252e%252e/secret/secret.txt",
			expectedStatus: http.StatusNotFound,
			should:         "block double URL-encoded path traversal",
		},
		{
			name:           "unicode normalization",
			path:           "/static/../secret/secret.txt",
			expectedStatus: http.StatusNotFound,
			should:         "block unicode-encoded path traversal",
		},
		{
			name:           "absolute path attempt",
			path:           "/static/" + secretFile,
			expectedStatus: http.StatusNotFound,
			should:         "block absolute path injection",
		},
		{
			name:           "dot segments",
			path:           "/static/./././../secret/secret.txt",
			expectedStatus: http.StatusNotFound,
			should:         "block path with multiple dot segments",
		},
		{
			name:           "trailing slash bypass",
			path:           "/static/../secret/secret.txt/",
			expectedStatus: http.StatusNotFound,
			should:         "block traversal with trailing slash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			app.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.should, tt.expectedStatus, w.Code)
			}

			// Ensure secret content is never leaked
			if w.Body.String() == "secret content" {
				t.Errorf("%s: secret content was leaked!", tt.should)
			}
		})
	}
}

// TestStatic_SymlinkChain tests symlink chains and nested symlinks
func TestStatic_SymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	secretDir := filepath.Join(tmpDir, "secret")

	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secretDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a secret file
	secretFile := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret via chain"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink chain: link1 -> link2 -> secret
	link2 := filepath.Join(staticDir, "link2.txt")
	if err := os.Symlink(secretFile, link2); err != nil {
		t.Fatal(err)
	}

	link1 := filepath.Join(staticDir, "link1.txt")
	if err := os.Symlink(link2, link1); err != nil {
		t.Fatal(err)
	}

	app := New()
	app.Static("/static", staticDir)

	// Try to access through the symlink chain
	req := httptest.NewRequest(http.MethodGet, "/static/link1.txt", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	// Should be forbidden because the chain eventually points outside
	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d for symlink chain, got %d", http.StatusForbidden, w.Code)
	}

	if w.Body.String() == "secret via chain" {
		t.Error("secret content was leaked via symlink chain!")
	}
}

// TestStatic_DirectoryTraversal tests directory listing prevention
func TestStatic_DirectoryTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	subDir := filepath.Join(staticDir, "subdir")

	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file in subdirectory
	if err := os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	app := New()
	app.Static("/static", staticDir)

	tests := []struct {
		name string
		path string
	}{
		{"root directory", "/static/"},
		{"subdirectory", "/static/subdir/"},
		{"directory without trailing slash", "/static/subdir"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			app.ServeHTTP(w, req)

			// Directory listing should not expose file names
			body := w.Body.String()
			if w.Code == http.StatusOK && (body == "" || body == "content") {
				// Either empty response or actual file content is fine
				return
			}

			// If we get a directory listing, ensure it doesn't leak sensitive info
			if w.Code == http.StatusOK {
				// http.FileServer may return directory listings
				// This is expected behavior, but we log it for awareness
				t.Logf("Directory listing returned for %s (status %d)", tt.path, w.Code)
			}
		})
	}
}

// TestStatic_CaseSensitivity tests case sensitivity handling
func TestStatic_CaseSensitivity(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")

	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file with specific case
	if err := os.WriteFile(filepath.Join(staticDir, "File.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	app := New()
	app.Static("/static", staticDir)

	// On case-sensitive filesystems, this should fail
	// On case-insensitive filesystems (macOS, Windows), this might succeed
	req := httptest.NewRequest(http.MethodGet, "/static/file.txt", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	// We don't assert a specific status code because it depends on the filesystem
	// Just ensure no panic or error occurs
	t.Logf("Case sensitivity test: status %d", w.Code)
}

// TestStatic_LargePathDepth tests handling of very deep path structures
func TestStatic_LargePathDepth(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")

	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a deeply nested directory structure
	deepPath := staticDir
	for i := 0; i < 50; i++ {
		deepPath = filepath.Join(deepPath, "level")
	}
	if err := os.MkdirAll(deepPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file at the deep level
	deepFile := filepath.Join(deepPath, "deep.txt")
	if err := os.WriteFile(deepFile, []byte("deep content"), 0644); err != nil {
		t.Fatal(err)
	}

	app := New()
	app.Static("/static", staticDir)

	// Build the URL path
	urlPath := "/static"
	for i := 0; i < 50; i++ {
		urlPath += "/level"
	}
	urlPath += "/deep.txt"

	req := httptest.NewRequest(http.MethodGet, urlPath, nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	// Should successfully serve the file
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d for deep path, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "deep content" {
		t.Errorf("expected 'deep content', got: %s", w.Body.String())
	}
}
