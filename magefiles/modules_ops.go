//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// allModulesOrdered is the canonical topological order used by both Tidy and AffectedModules.
// Kept in one place to avoid the drift that existed between tidy-all.sh and affected-modules.sh.
var allModulesOrdered = []string{
	".",
	"otel", "mq", "taskqueue", "storage", "discovery", "config",
	"cache", "lock", "search", "notify", "mongodb", "lua",
	"orm", "grpc", "session", "auth",
	"runner", "client", "testutil",
}

// modDeps maps a child module to the workspace modules it depends on.
var modDeps = map[string][]string{
	"orm":      {"."},
	"grpc":     {"."},
	"session":  {"."},
	"auth":     {"."},
	"runner":   {".", "taskqueue"},
	"client":   {".", "discovery"},
	"testutil": {".", "cache"},
}

// AffectedModules prints the workspace modules affected by the current branch
// compared to origin/main, including transitive downstream dependents.
// Set BASE env var to compare against a different ref. Set ALL=1 for full output.
func AffectedModules() error {
	if os.Getenv("ALL") == "1" {
		for _, m := range allModulesOrdered {
			fmt.Println(m)
		}
		return nil
	}

	base := os.Getenv("BASE")
	if base == "" {
		base = "origin/main"
	}

	// Check if base ref exists.
	if err := exec.Command("git", "rev-parse", base).Run(); err != nil {
		// Can't resolve ref — output all modules.
		for _, m := range allModulesOrdered {
			fmt.Println(m)
		}
		return nil
	}

	// Get changed files (three-dot diff, fallback to two-dot for shallow clones).
	changed, err := changedFiles(base)
	if err != nil || len(changed) == 0 {
		for _, m := range allModulesOrdered {
			fmt.Println(m)
		}
		return nil
	}

	dirty := map[string]bool{}
	for _, f := range changed {
		if f == "go.work" || f == "go.work.sum" {
			for _, m := range allModulesOrdered {
				dirty[m] = true
			}
			break
		}
		dirty[ownerOf(f)] = true
	}

	// BFS: if a parent is dirty, mark its children dirty too.
	expandDeps(dirty)

	// Print in topological order.
	for _, m := range allModulesOrdered {
		if dirty[m] {
			fmt.Println(m)
		}
	}
	return nil
}

func changedFiles(base string) ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", base+"...HEAD").Output()
	if err != nil {
		out, err = exec.Command("git", "diff", "--name-only", base, "HEAD").Output()
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func ownerOf(file string) string {
	best := "."
	for _, mod := range allModulesOrdered {
		if mod == "." {
			continue
		}
		if (strings.HasPrefix(file, mod+"/") || file == mod) && len(mod) > len(best) {
			best = mod
		}
	}
	return best
}

func expandDeps(dirty map[string]bool) {
	changed := true
	for changed {
		changed = false
		for child, parents := range modDeps {
			if dirty[child] {
				continue
			}
			for _, p := range parents {
				if dirty[p] {
					dirty[child] = true
					changed = true
					break
				}
			}
		}
	}
}

// CheckDepVersions detects dependencies used at different versions across workspace modules.
func CheckDepVersions() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	// module path → (version → []module dirs that use it)
	type versionSeen struct {
		version string
		modDir  string
	}
	firstSeen := map[string]versionSeen{}  // dep → first (version, modDir)
	conflicts := map[string][]versionSeen{} // dep → all conflicting entries

	mods, err := listModules(root, false)
	if err != nil {
		return err
	}

	for _, mod := range mods {
		dir := root
		if mod != "." {
			dir = filepath.Join(root, mod)
		}
		gomod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(gomod); os.IsNotExist(err) {
			continue
		}

		out, err := exec.Command("go", "mod", "edit", "-json", gomod).Output()
		if err != nil {
			continue
		}

		deps := parseRequires(out)
		for dep, ver := range deps {
			// skip intra-workspace pseudo-versions
			if strings.HasPrefix(ver, "v0.0.0-00010101") {
				continue
			}
			if prev, ok := firstSeen[dep]; !ok {
				firstSeen[dep] = versionSeen{ver, mod}
			} else if prev.version != ver {
				if _, already := conflicts[dep]; !already {
					conflicts[dep] = []versionSeen{prev}
				}
				conflicts[dep] = append(conflicts[dep], versionSeen{ver, mod})
			}
		}
	}

	if len(conflicts) == 0 {
		fmt.Println("✓ All external dependencies use consistent versions across the workspace")
		return nil
	}

	fail := false
	for dep, entries := range conflicts {
		fmt.Printf("✗ %s\n", dep)
		for _, e := range entries {
			fmt.Printf("    %s  (%s)\n", e.version, e.modDir)
		}
		fail = true
	}
	if fail {
		return fmt.Errorf("version conflicts detected")
	}
	return nil
}
