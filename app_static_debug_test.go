package astra

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatic_SymlinkDebug(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	subDir := filepath.Join(staticDir, "subdir")

	if err := os.MkdirAll(subDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create target file
	targetFile := filepath.Join(subDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink within static directory
	linkPath := filepath.Join(staticDir, "link.txt")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		t.Fatal(err)
	}

	// Debug: Print paths
	absRoot, _ := filepath.Abs(staticDir)
	t.Logf("absRoot: %s", absRoot)
	t.Logf("linkPath: %s", linkPath)
	t.Logf("targetFile: %s", targetFile)

	// Simulate the logic in Static handler
	reqPath := "link.txt"
	cleanPath := filepath.Clean("/" + reqPath)
	t.Logf("cleanPath: %s", cleanPath)

	fullPath := filepath.Join(absRoot, cleanPath)
	t.Logf("fullPath: %s", fullPath)

	fullPathNorm := filepath.Clean(fullPath)
	absRootNorm := filepath.Clean(absRoot)
	t.Logf("fullPathNorm: %s", fullPathNorm)
	t.Logf("absRootNorm: %s", absRootNorm)

	// First check
	relPath, err := filepath.Rel(absRootNorm, fullPathNorm)
	t.Logf("First check - relPath: %s, err: %v", relPath, err)
	t.Logf("First check - HasPrefix '..'?: %v", strings.HasPrefix(relPath, ".."))

	if err != nil || strings.HasPrefix(relPath, "..") {
		t.Logf("BLOCKED at first check")
		return
	}

	// Check if it's a symlink
	info, err := os.Lstat(fullPath)
	if err != nil {
		t.Logf("Lstat error: %v", err)
		return
	}
	t.Logf("Is symlink?: %v", info.Mode()&os.ModeSymlink != 0)

	if info.Mode()&os.ModeSymlink != 0 {
		resolvedPath, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			t.Logf("EvalSymlinks error: %v", err)
			return
		}
		t.Logf("resolvedPath: %s", resolvedPath)

		resolvedPathNorm := filepath.Clean(resolvedPath)
		t.Logf("resolvedPathNorm: %s", resolvedPathNorm)

		relPath2, err := filepath.Rel(absRootNorm, resolvedPathNorm)
		t.Logf("Second check - relPath: %s, err: %v", relPath2, err)
		t.Logf("Second check - HasPrefix '..'?: %v", strings.HasPrefix(relPath2, ".."))

		if err != nil || strings.HasPrefix(relPath2, "..") {
			t.Logf("BLOCKED at second check")
			return
		}
	}

	t.Logf("PASSED all checks")

	// Now test with actual app
	app := New()
	app.Static("/static", staticDir)

	req := httptest.NewRequest(http.MethodGet, "/static/link.txt", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
