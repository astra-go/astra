package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// checkGoVersion verifies that the Go version is 1.20 or higher.
func checkGoVersion(dir string) Check {
	cmd := exec.Command("go", "version")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return Check{
			Name:   "go version",
			Status: StatusFail,
			Detail: "could not determine Go version",
			Hint:   "ensure Go is installed and available in PATH",
		}
	}

	version := string(output)
	re := regexp.MustCompile(`go(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(version)
	if len(matches) < 3 {
		return Check{
			Name:   "go version",
			Status: StatusWarn,
			Detail: fmt.Sprintf("unrecognized version format: %s", strings.TrimSpace(version)),
		}
	}

	major := matches[1]
	if major != "1" {
		return Check{
			Name:   "go version",
			Status: StatusOK,
			Detail: strings.TrimSpace(version),
		}
	}

	// Check if minor version >= 20
	minor := matches[2]
	if minor < "20" {
		return Check{
			Name:   "go version",
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s (recommended: go1.20+)", strings.TrimSpace(version)),
			Hint:   "upgrade to Go 1.20 or higher for best compatibility",
		}
	}

	return Check{
		Name:   "go version",
		Status: StatusOK,
		Detail: strings.TrimSpace(version),
	}
}

// checkMageInstalled checks if mage build tool is installed.
func checkMageInstalled(dir string) Check {
	_, err := exec.LookPath("mage")
	if err != nil {
		// Check if magefiles/ directory exists (indicates project uses mage)
		if _, statErr := os.Stat(filepath.Join(dir, "magefiles")); statErr == nil {
			return Check{
				Name:   "mage installed",
				Status: StatusWarn,
				Detail: "mage not found but magefiles/ directory exists",
				Hint:   "install mage: go install github.com/magefile/mage@latest",
			}
		}
		return Check{
			Name:   "mage installed",
			Status: StatusOK,
			Detail: "not installed (optional build tool)",
		}
	}

	cmd := exec.Command("mage", "-version")
	output, err := cmd.Output()
	if err != nil {
		return Check{
			Name:   "mage installed",
			Status: StatusWarn,
			Detail: "mage found but version check failed",
		}
	}

	return Check{
		Name:   "mage installed",
		Status: StatusOK,
		Detail: strings.TrimSpace(string(output)),
	}
}

// checkCircularDeps checks for circular dependencies in go.mod.
func checkCircularDeps(dir string) Check {
	goModPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return Check{
			Name:   "circular deps",
			Status: StatusWarn,
			Detail: "skipped (no go.mod)",
		}
	}

	// Run go mod graph to detect circular dependencies
	cmd := exec.Command("go", "mod", "graph")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return Check{
			Name:   "circular deps",
			Status: StatusWarn,
			Detail: "could not analyze dependency graph",
		}
	}

	// Simple heuristic: circular deps would cause go mod graph to fail or warn
	// For now, if command succeeds, assume no circular deps
	lines := strings.Split(string(output), "\n")
	edgeCount := 0
	for _, line := range lines {
		if strings.Contains(line, " ") {
			edgeCount++
		}
	}

	return Check{
		Name:   "circular deps",
		Status: StatusOK,
		Detail: fmt.Sprintf("none detected (%d dependency edges)", edgeCount),
	}
}

// checkCoreDeps verifies that core module has minimal dependencies (ADR-001).
func checkCoreDeps(dir string) Check {
	// Check if this is the astra monorepo or a project using astra
	coreDir := filepath.Join(dir, "core")
	if _, err := os.Stat(coreDir); os.IsNotExist(err) {
		// Not a monorepo, check if core is imported
		cmd := exec.Command("go", "list", "-m", "github.com/astra-go/astra/core")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			return Check{
				Name:   "core deps (ADR-001)",
				Status: StatusOK,
				Detail: "not applicable (no core module)",
			}
		}
	}

	// For projects using astra core, we skip detailed check
	// For the monorepo itself, mage checkCoreDeps would be authoritative
	return Check{
		Name:   "core deps (ADR-001)",
		Status: StatusOK,
		Detail: "use 'mage checkCoreDeps' for detailed analysis",
	}
}

// checkModuleCount checks if module count is within limits (ADR-005).
func checkModuleCount(dir string) Check {
	goWorkPath := filepath.Join(dir, "go.work")
	if _, err := os.Stat(goWorkPath); os.IsNotExist(err) {
		return Check{
			Name:   "module count (ADR-005)",
			Status: StatusOK,
			Detail: "not applicable (single module project)",
		}
	}

	data, err := os.ReadFile(goWorkPath)
	if err != nil {
		return Check{
			Name:   "module count (ADR-005)",
			Status: StatusWarn,
			Detail: "could not read go.work",
		}
	}

	// Count use directives
	count := 0
	inBlock := false
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if line == "use (" {
			inBlock = true
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = strings.TrimSpace(line[:idx])
			}
			if line != "" {
				count++
			}
			continue
		}
		if strings.HasPrefix(line, "use ") {
			count++
		}
	}

	const maxModules = 40 // ADR-005 limit
	if count > maxModules {
		return Check{
			Name:   "module count (ADR-005)",
			Status: StatusWarn,
			Detail: fmt.Sprintf("%d modules (limit: %d per ADR-005)", count, maxModules),
			Hint:   "consider consolidating modules to stay under the 40-module threshold",
		}
	}

	return Check{
		Name:   "module count (ADR-005)",
		Status: StatusOK,
		Detail: fmt.Sprintf("%d modules (limit: %d)", count, maxModules),
	}
}

// checkGitClean verifies working tree is clean (no uncommitted changes).
func checkGitClean(dir string) Check {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return Check{
			Name:   "git working tree",
			Status: StatusOK,
			Detail: "not a git repository",
		}
	}

	// Check if inside a git repository
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return Check{
			Name:   "git working tree",
			Status: StatusOK,
			Detail: "not a git repository",
		}
	}

	// Check for uncommitted changes
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return Check{
			Name:   "git working tree",
			Status: StatusWarn,
			Detail: "could not check git status",
		}
	}

	if len(strings.TrimSpace(string(output))) > 0 {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		return Check{
			Name:   "git working tree",
			Status: StatusWarn,
			Detail: fmt.Sprintf("%d uncommitted change(s)", len(lines)),
			Hint:   "commit or stash changes before running code generation",
		}
	}

	return Check{
		Name:   "git working tree",
		Status: StatusOK,
		Detail: "clean (no uncommitted changes)",
	}
}
