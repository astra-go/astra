package astra

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStatic_PathTraversal(t *testing.T) {
	// Create a temporary directory structure for testing
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
		description    string
	}{
		{
			name:           "valid file access",
			path:           "/static/public.txt",
			expectedStatus: http.StatusOK,
			description:    "should allow access to files within static directory",
		},
		{
			name:           "path traversal with ../",
			path:           "/static/../secret/secret.txt",
			expectedStatus: http.StatusNotFound, // 404 is acceptable - file doesn't exist after normalization
			description:    "should block path traversal attempts using ../",
		},
		{
			name:           "multiple path traversal",
			path:           "/static/../../secret/secret.txt",
			expectedStatus: http.StatusNotFound, // 404 is acceptable - file doesn't exist after normalization
			description:    "should block multiple ../ traversal attempts",
		},
		{
			name:           "encoded path traversal",
			path:           "/static/%2e%2e/secret/secret.txt",
			expectedStatus: http.StatusNotFound, // 404 is acceptable - file doesn't exist after normalization
			description:    "should block URL-encoded path traversal",
		},
		{
			name:           "non-existent file",
			path:           "/static/nonexistent.txt",
			expectedStatus: http.StatusNotFound,
			description:    "should return 404 for non-existent files",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			
			app.ServeHTTP(w, req)
			
			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.description, tt.expectedStatus, w.Code)
			}
			
			// Ensure secret content is never leaked
			if w.Code == http.StatusOK && w.Body.String() == "secret content" {
				t.Errorf("%s: secret content was leaked!", tt.description)
			}
		})
	}
}

func TestStatic_SymlinkTraversal(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	secretDir := filepath.Join(tmpDir, "secret")
	
	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secretDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	// Create a secret file outside static directory
	secretFile := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret via symlink"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create a symlink inside static directory pointing to secret file
	symlinkPath := filepath.Join(staticDir, "link.txt")
	if err := os.Symlink(secretFile, symlinkPath); err != nil {
		t.Skip("symlink creation not supported on this platform")
	}
	
	app := New()
	app.Static("/static", staticDir)
	
	// Try to access the symlink
	req := httptest.NewRequest(http.MethodGet, "/static/link.txt", nil)
	w := httptest.NewRecorder()
	
	app.ServeHTTP(w, req)
	
	// Should be forbidden because symlink points outside static directory
	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d for symlink traversal, got %d", http.StatusForbidden, w.Code)
	}
	
	// Ensure secret content is not leaked
	if w.Body.String() == "secret via symlink" {
		t.Error("secret content was leaked via symlink!")
	}
}

func TestStatic_ValidSymlinkWithinRoot(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	subDir := filepath.Join(staticDir, "subdir")
	
	if err := os.Mkdir(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	// Create a file in subdirectory
	targetFile := filepath.Join(subDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("valid symlink content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create a symlink within static directory pointing to another file in static directory
	symlinkPath := filepath.Join(staticDir, "link.txt")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skip("symlink creation not supported on this platform")
	}
	
	app := New()
	app.Static("/static", staticDir)
	
	// Try to access the symlink
	req := httptest.NewRequest(http.MethodGet, "/static/link.txt", nil)
	w := httptest.NewRecorder()
	
	app.ServeHTTP(w, req)
	
	// Should be allowed because symlink points within static directory
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d for valid symlink, got %d", http.StatusOK, w.Code)
	}
	
	if w.Body.String() != "valid symlink content" {
		t.Errorf("expected valid symlink content, got: %s", w.Body.String())
	}
}
