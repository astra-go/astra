//go:build mage

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// listModules reads go.work and returns all workspace module dirs relative to root.
// Root module is returned as ".". Excludes examples/* when excludeExamples is true.
func listModules(root string, excludeExamples bool) ([]string, error) {
	gowork := filepath.Join(root, "go.work")
	f, err := os.Open(gowork)
	if err != nil {
		return nil, fmt.Errorf("open go.work: %w", err)
	}
	defer f.Close()

	var mods []string
	inUse := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// strip inline comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "use (") || line == "use (" {
			inUse = true
			continue
		}
		if inUse && line == ")" {
			inUse = false
			continue
		}
		if !inUse {
			continue
		}
		// normalize: "./foo" → "foo", "." → "."
		mod := strings.TrimPrefix(line, "./")
		if mod == "" {
			mod = "."
		}
		if excludeExamples && strings.HasPrefix(mod, "examples/") {
			continue
		}
		mods = append(mods, mod)
	}
	return mods, scanner.Err()
}

// modPath reads the module path from a go.mod file.
func modPath(gomod string) (string, error) {
	f, err := os.Open(gomod)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module directive not found in %s", gomod)
}

// repoRoot returns the absolute path to the repository root (where the monorepo
// go.work lives). It skips go.work files that only contain a single "use ." entry
// (e.g. magefiles/go.work) and keeps walking up until it finds the workspace root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		gowork := filepath.Join(dir, "go.work")
		if _, err := os.Stat(gowork); err == nil {
			if isMonorepoWorkspace(gowork) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("monorepo go.work not found in any parent directory")
		}
		dir = parent
	}
}

// isMonorepoWorkspace returns true if the go.work file lists more than one module,
// distinguishing the real workspace root from leaf go.work files like magefiles/go.work.
func isMonorepoWorkspace(gowork string) bool {
	mods, err := listModules(filepath.Dir(gowork), false)
	return err == nil && len(mods) > 1
}
