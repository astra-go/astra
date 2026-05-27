//go:build mage

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// exemptGoVersionModules lists workspace-relative module dirs that are allowed
// to declare a Go version lower than the core minimum. These are standalone
// examples that intentionally target older Go for compatibility demos.
var exemptGoVersionModules = map[string]bool{
	"examples/showcase": true,
	"examples/wasm":     true,
}

// CheckGoVersions verifies that every non-exempt workspace module declares a
// Go version >= the core module's minimum. Exits with an error listing any
// modules that are out of sync.
//
// Run this in CI to prevent version drift across the monorepo.
func CheckGoVersions() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	baseline, err := readGoVersion(filepath.Join(root, "go.mod"))
	if err != nil {
		return fmt.Errorf("read core go.mod: %w", err)
	}

	mods, err := listModules(root, false)
	if err != nil {
		return err
	}

	var violations []string
	for _, mod := range mods {
		if mod == "." {
			continue
		}
		if exemptGoVersionModules[mod] {
			continue
		}
		gomod := filepath.Join(root, mod, "go.mod")
		if _, err := os.Stat(gomod); os.IsNotExist(err) {
			continue
		}
		ver, err := readGoVersion(gomod)
		if err != nil {
			violations = append(violations, fmt.Sprintf("  %-40s  (could not read version: %v)", mod, err))
			continue
		}
		if compareGoVersion(ver, baseline) < 0 {
			violations = append(violations, fmt.Sprintf("  %-40s  go %s  (want >= go %s)", mod, ver, baseline))
		}
	}

	if len(violations) == 0 {
		fmt.Printf("✓ All %d modules declare go >= %s\n", len(mods), baseline)
		return nil
	}

	fmt.Printf("✗ %d module(s) declare a Go version below the core minimum (go %s):\n\n", len(violations), baseline)
	for _, v := range violations {
		fmt.Println(v)
	}
	fmt.Println()
	fmt.Println("Fix: update the go directive in each listed go.mod to match the core version,")
	fmt.Println("     then run: mage tidy")
	return fmt.Errorf("go version drift detected")
}

// readGoVersion returns the version string from the `go X.Y.Z` directive in a go.mod file.
func readGoVersion(gomod string) (string, error) {
	f, err := os.Open(gomod)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "go ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no go directive found in %s", gomod)
}

// compareGoVersion compares two Go version strings (e.g. "1.25.1" vs "1.25.0").
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareGoVersion(a, b string) int {
	pa := parseGoVersionParts(a)
	pb := parseGoVersionParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func parseGoVersionParts(v string) [3]int {
	var parts [3]int
	segments := strings.SplitN(v, ".", 3)
	for i, s := range segments {
		if i >= 3 {
			break
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				break
			}
			parts[i] = parts[i]*10 + int(c-'0')
		}
	}
	return parts
}
