//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const zeroVer = "v0.0.0-00010101000000-000000000000"

// SyncReplaces syncs intra-workspace replace directives across all go.mod files.
// Every go.mod gets a replace entry for every other workspace module, pointing
// to its local path. This lets `go mod tidy` resolve intra-workspace deps
// without hitting VCS.
func SyncReplaces() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	table, err := buildModTable(root)
	if err != nil {
		return err
	}
	return applyReplaces(root, table, root)
}

// CheckReplaces verifies that all go.mod intra-workspace replace directives are
// in sync. Exits with an error if any drift is detected.
// Run SyncReplaces to fix drift automatically.
func CheckReplaces() error {
	return checkReplaces(false)
}

// FixReplaces checks for replace drift and automatically runs SyncReplaces if found.
func FixReplaces() error {
	return checkReplaces(true)
}

func checkReplaces(fix bool) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	// Build expected state in a scratch directory.
	scratch, err := os.MkdirTemp("", "astra-check-replaces-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)

	// Copy all go.mod files into scratch, preserving relative paths.
	if err := copyGoMods(root, scratch); err != nil {
		return err
	}

	// Build the module table from the scratch copy.
	table, err := buildModTable(scratch)
	if err != nil {
		return err
	}

	// Apply sync logic to scratch tree.
	if err := applyReplaces(scratch, table, scratch); err != nil {
		return err
	}

	// Compare replace blocks between original and scratch.
	driftFiles, err := findDrift(root, scratch)
	if err != nil {
		return err
	}

	if len(driftFiles) == 0 {
		fmt.Println("✓ all go.mod intra-workspace replace directives are in sync")
		return nil
	}

	fmt.Printf("✗ intra-workspace replace drift detected in %d file(s):\n", len(driftFiles))
	for _, f := range driftFiles {
		fmt.Printf("  %s\n", f)
	}
	fmt.Println()
	fmt.Println("Fix: mage syncReplaces")

	if fix {
		fmt.Println()
		fmt.Println("Running SyncReplaces ...")
		if err := SyncReplaces(); err != nil {
			return err
		}
		fmt.Println("✓ fixed")
		return nil
	}
	return fmt.Errorf("replace drift detected")
}

// DropReplaces removes all intra-workspace replace directives from every go.mod.
// Run this before publishing a release.
func DropReplaces() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	table, err := buildModTable(root)
	if err != nil {
		return err
	}

	// Build the -dropreplace args.
	dropArgs := make([]string, 0, len(table))
	for modPath := range table {
		dropArgs = append(dropArgs, fmt.Sprintf("-dropreplace=%s@%s", modPath, zeroVer))
	}

	cleaned, skipped := 0, 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "astra-go/astra") {
			skipped++
			return nil
		}
		dir := filepath.Dir(path)
		args := append([]string{"mod", "edit"}, dropArgs...)
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go mod edit in %s: %w", dir, err)
		}
		rel, _ := filepath.Rel(root, path)
		fmt.Printf("✓ cleaned: %s\n", rel)
		cleaned++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("✓ 完成：清理 %d 个 go.mod，跳过 %d 个（已干净）\n", cleaned, skipped)
	return nil
}

// buildModTable returns a map of module-path → workspace-relative-dir for all
// workspace modules. Root module maps to ".".
func buildModTable(root string) (map[string]string, error) {
	mods, err := listModules(root, false)
	if err != nil {
		return nil, err
	}
	table := make(map[string]string, len(mods))
	for _, mod := range mods {
		dir := root
		if mod != "." {
			dir = filepath.Join(root, mod)
		}
		gomod := filepath.Join(dir, "go.mod")
		mp, err := modPath(gomod)
		if err != nil {
			continue
		}
		// skip example/* module paths
		if strings.HasPrefix(mp, "example/") {
			continue
		}
		table[mp] = mod
	}
	return table, nil
}

// applyReplaces writes replace directives into every go.mod under treeRoot,
// using module paths from table and resolving local paths relative to treeRoot.
func applyReplaces(treeRoot string, table map[string]string, _ string) error {
	synced := 0
	err := filepath.WalkDir(treeRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}

		dir := filepath.Dir(path)
		ownPath, err := modPath(path)
		if err != nil {
			return nil
		}

		n := 0
		for depPath, depDir := range table {
			if depPath == ownPath {
				continue
			}
			// Compute relative path from this go.mod's dir to the dep's dir.
			var depAbsDir string
			if depDir == "." {
				depAbsDir = treeRoot
			} else {
				depAbsDir = filepath.Join(treeRoot, depDir)
			}
			rel, err := filepath.Rel(dir, depAbsDir)
			if err != nil {
				continue
			}
			// go mod edit requires local paths to start with ./ or ../
			if !strings.HasPrefix(rel, ".") {
				rel = "./" + rel
			}
			replaceArg := fmt.Sprintf("-replace=%s@%s=%s", depPath, zeroVer, rel)
			cmd := exec.Command("go", "mod", "edit", replaceArg, filepath.Base(path))
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("go mod edit in %s: %w\n%s", dir, err, out)
			}
			n++
		}

		rel, _ := filepath.Rel(treeRoot, path)
		fmt.Printf("✓ synced: %s (%d replace 指令)\n", rel, n)
		synced++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("\n✓ 完成：同步 %d 个 go.mod\n", synced)
	return nil
}

// copyGoMods copies all go.mod files from src tree into dst tree, preserving
// relative paths. Also copies go.work so listModules works on the scratch tree.
func copyGoMods(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" && d.Name() != "go.work" {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		dst2 := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(dst2), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst2, data, 0o644)
	})
}

// findDrift compares the replace blocks in original go.mod files against the
// expected state in the scratch tree. Returns relative paths of drifted files.
func findDrift(original, scratch string) ([]string, error) {
	var drifted []string
	err := filepath.WalkDir(original, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		rel, _ := filepath.Rel(original, path)
		scratchPath := filepath.Join(scratch, rel)
		if _, err := os.Stat(scratchPath); os.IsNotExist(err) {
			return nil
		}

		origReplaces := extractIntraReplaces(path, original)
		scratchReplaces := extractIntraReplaces(scratchPath, scratch)

		if !stringSlicesEqual(origReplaces, scratchReplaces) {
			drifted = append(drifted, rel)
		}
		return nil
	})
	return drifted, err
}

// extractIntraReplaces returns sorted canonical replace entries for astra-go/astra
// modules from a go.mod file. Paths are resolved to root-relative form.
func extractIntraReplaces(gomod, treeRoot string) []string {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return nil
	}
	gomodDir := filepath.Dir(gomod)
	var result []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "github.com/astra-go/astra") || !strings.Contains(line, "=>") {
			continue
		}
		// Extract the local path after "=>"
		parts := strings.SplitN(line, "=>", 2)
		if len(parts) != 2 {
			continue
		}
		localPath := strings.TrimSpace(parts[1])
		// Resolve to absolute, then make root-relative.
		abs := filepath.Join(gomodDir, localPath)
		rootRel, err := filepath.Rel(treeRoot, abs)
		if err != nil {
			rootRel = localPath
		}
		modPart := strings.TrimSpace(parts[0])
		result = append(result, modPart+" => "+rootRel)
	}
	sortStrings(result)
	return result
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
