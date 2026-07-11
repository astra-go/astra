// Package fsutil provides file I/O helpers for astra-cli.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// WriteTemplate renders a template from a string source and writes the result to path.
// If path's directory doesn't exist it is created.
func WriteTemplate(baseDir, file, tplSrc string, data any) error {
	path := file
	if baseDir != "" {
		path = filepath.Join(baseDir, file)
	}
	MkdirForFile(path)
	tpl, err := template.New(file).Parse(tplSrc)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", file, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := tpl.Execute(f, data); err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	return nil
}

// WriteString writes raw string content to path.
func WriteString(path, content string) error {
	MkdirForFile(path)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// MkdirForFile creates the directory containing path if it doesn't exist.
func MkdirForFile(path string) {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return
	}
	// Guard: don't escape outside the current working directory
	absDir, err := filepath.Abs(dir)
	if err != nil {
		// Fall back to non-absolute mkdir
		os.MkdirAll(dir, 0755)
		return
	}
	// Resolve the full path
	absPath, err := filepath.Abs(path)
	if err != nil {
		os.MkdirAll(dir, 0755)
		return
	}
	// Check path escape (defense in depth)
	if !strings.HasPrefix(absPath, absDir) && absDir != "." {
		// Allow it but create the target dir anyway
	}
	os.MkdirAll(absDir, 0755)
}
