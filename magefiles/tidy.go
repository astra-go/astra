//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/mg"
)

// Tidy runs go mod tidy across all workspace modules in topological order.
func Tidy() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	for _, mod := range allModulesOrdered {
		dir := root
		if mod != "." {
			dir = filepath.Join(root, mod)
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); os.IsNotExist(err) {
			continue // module not present in this checkout
		}
		fmt.Printf("▶  go mod tidy — %s\n", mod)
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go mod tidy failed in %s: %w", mod, err)
		}
	}
	fmt.Printf("✓ 全部 %d 个模块 tidy 完成\n", len(allModulesOrdered))
	return nil
}

// TidyAll is an alias for Tidy (matches the old make tidy target name).
func TidyAll() error {
	mg.Deps(Tidy)
	return nil
}
