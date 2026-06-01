package astra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSymlinkDebug(t *testing.T) {
	tmpDir := t.TempDir()
	staticDir := filepath.Join(tmpDir, "static")
	subDir := filepath.Join(staticDir, "subdir")
	
	os.Mkdir(staticDir, 0755)
	os.Mkdir(subDir, 0755)
	
	targetFile := filepath.Join(subDir, "target.txt")
	os.WriteFile(targetFile, []byte("valid symlink content"), 0644)
	
	symlinkPath := filepath.Join(staticDir, "link.txt")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Skip("symlink not supported")
	}
	
	// Simulate the logic
	absRoot := filepath.Clean(staticDir)
	fullPath := filepath.Join(absRoot, "link.txt")
	
	t.Logf("absRoot: %s", absRoot)
	t.Logf("fullPath: %s", fullPath)
	
	info, _ := os.Lstat(fullPath)
	if info.Mode()&os.ModeSymlink != 0 {
		resolvedPath, _ := filepath.EvalSymlinks(fullPath)
		resolvedPathNorm := filepath.Clean(resolvedPath)
		
		t.Logf("resolvedPath: %s", resolvedPath)
		t.Logf("resolvedPathNorm: %s", resolvedPathNorm)
		
		relPath, err := filepath.Rel(absRoot, resolvedPathNorm)
		t.Logf("relPath: %s, err: %v", relPath, err)
		t.Logf("hasPrefix ..: %v", strings.HasPrefix(relPath, ".."))
	}
}
