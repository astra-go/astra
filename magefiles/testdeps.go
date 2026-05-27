//go:build mage

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// exemptModules are test/benchmark/example modules — all their direct deps are intentional.
var exemptModules = map[string]bool{
	"benchmarks":        true,
	"e2e":               true,
	"e2e/orm":           true,
	"e2e/search":        true,
	"examples/crud":     true,
	"examples/orm":      true,
	"examples/showcase": true,
	"examples/wasm":     true,
}

// exemptPkgs are intentionally test-only direct deps.
var exemptPkgs = map[string]bool{
	"github.com/astra-go/astra/testutil":  true,
	"go.opentelemetry.io/otel/sdk/metric": true,
	"github.com/glebarez/sqlite":          true,
	"modernc.org/sqlite":                  true,
	"github.com/mattn/go-sqlite3":         true,
}

// heavyTestPkgPrefixes are always an error when used only in tests.
var heavyTestPkgPrefixes = []string{
	"github.com/testcontainers/",
	"github.com/stretchr/testify",
	"github.com/golang/mock",
	"go.uber.org/mock",
	"github.com/vektra/mockery",
	"github.com/DATA-DOG/go-sqlmock",
	"github.com/jarcoal/httpmock",
	"github.com/h2non/gock",
	"github.com/onsi/ginkgo",
	"github.com/onsi/gomega",
}

// lightTestPkgPrefixes are only flagged in strict mode.
var lightTestPkgPrefixes = []string{
	"github.com/glebarez/sqlite",
	"modernc.org/sqlite",
	"github.com/mattn/go-sqlite3",
}

// CheckTestDeps detects test-only packages incorrectly declared as direct
// (production) dependencies in go.mod.
func CheckTestDeps() error {
	return checkTestDeps(false)
}

// CheckTestDepsStrict is like CheckTestDeps but also flags lightweight test helpers.
func CheckTestDepsStrict() error {
	return checkTestDeps(true)
}

func checkTestDeps(strict bool) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	mods, err := listModules(root, false)
	if err != nil {
		return err
	}

	errorCount, warnCount := 0, 0

	for _, mod := range mods {
		if exemptModules[mod] {
			continue
		}
		dir := root
		if mod != "." {
			dir = filepath.Join(root, mod)
		}
		gomod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(gomod); os.IsNotExist(err) {
			continue
		}

		prodImports, testImports, err := getImports(dir)
		if err != nil {
			continue
		}

		directDeps, err := getDirectDeps(gomod)
		if err != nil {
			continue
		}

		annotated, err := getAnnotatedTestOnly(gomod)
		if err != nil {
			continue
		}

		for pkg, ver := range directDeps {
			// skip intra-workspace pseudo-versions
			if strings.HasPrefix(ver, "v0.0.0-00010101") {
				continue
			}
			if exemptPkgs[pkg] || annotated[pkg] {
				continue
			}
			// used in production code?
			if hasPrefix(prodImports, pkg) {
				continue
			}
			// used in tests at all?
			if !hasPrefix(testImports, pkg) {
				continue
			}

			severity := classifyPkg(pkg, strict)
			switch severity {
			case "heavy":
				fmt.Printf("✗ [%s] %s\n", mod, pkg)
				fmt.Printf("      declared as direct dep but only imported in _test.go files\n")
				fmt.Printf("      fix: move to a dedicated e2e sub-module, or mark as '// indirect'\n")
				errorCount++
			case "light", "unknown":
				if strict {
					fmt.Printf("⚠ [%s] %s\n", mod, pkg)
					fmt.Printf("      declared as direct dep but only imported in _test.go files\n")
					warnCount++
				}
			}
		}
	}

	fmt.Println()
	if errorCount == 0 && warnCount == 0 {
		fmt.Println("✓ All modules pass dependency health check")
		return nil
	}

	total := errorCount + warnCount
	fmt.Printf("Found %d issue(s): %d error(s), %d warning(s)\n", total, errorCount, warnCount)
	fmt.Println()
	fmt.Println("How to fix:")
	fmt.Println("  Option A — annotate as intentional in go.mod:")
	fmt.Println("    require github.com/some/pkg v1.2.3 // test-only")
	fmt.Println("  Option B — isolate into a dedicated e2e sub-module")
	fmt.Println("  Option C — verify the package is truly needed at build time")
	if errorCount > 0 {
		return fmt.Errorf("%d test-dep violation(s) found", errorCount)
	}
	return nil
}

type goListPkg struct {
	Imports      []string
	TestImports  []string
	XTestImports []string
}

func getImports(dir string) (prod, test map[string]bool, err error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}

	prod = map[string]bool{}
	test = map[string]bool{}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var pkg goListPkg
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		for _, imp := range pkg.Imports {
			prod[imp] = true
		}
		for _, imp := range pkg.TestImports {
			test[imp] = true
		}
		for _, imp := range pkg.XTestImports {
			test[imp] = true
		}
	}
	return prod, test, nil
}

type goModRequire struct {
	Path     string
	Version  string
	Indirect bool
}

type goModEditJSON struct {
	Require []goModRequire
}

func getDirectDeps(gomod string) (map[string]string, error) {
	out, err := exec.Command("go", "mod", "edit", "-json", gomod).Output()
	if err != nil {
		return nil, err
	}
	var m goModEditJSON
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, r := range m.Require {
		if !r.Indirect {
			result[r.Path] = r.Version
		}
	}
	return result, nil
}

// getAnnotatedTestOnly returns the set of packages annotated with "// test-only"
// in the given go.mod.
func getAnnotatedTestOnly(gomod string) (map[string]bool, error) {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "// test-only") {
			// extract the package path (first token after leading whitespace)
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				result[fields[0]] = true
			}
		}
	}
	return result, nil
}

func hasPrefix(imports map[string]bool, pkg string) bool {
	for imp := range imports {
		if imp == pkg || strings.HasPrefix(imp, pkg+"/") {
			return true
		}
	}
	return false
}

func classifyPkg(pkg string, strict bool) string {
	for _, p := range heavyTestPkgPrefixes {
		if pkg == p || strings.HasPrefix(pkg, p) || strings.HasPrefix(pkg, strings.TrimSuffix(p, "/")) {
			return "heavy"
		}
	}
	for _, p := range lightTestPkgPrefixes {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return "light"
		}
	}
	if strict {
		return "unknown"
	}
	return ""
}
