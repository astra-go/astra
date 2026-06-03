package doctor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Status values for a check result.
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// Check holds the result of one diagnostic check.
type Check struct {
	Name   string
	Status string
	Detail string
	Hint   string
}

// Run performs all diagnostic checks in dir and returns the results.
// Checks are executed in parallel for better performance.
func Run(dir string) []Check {
	checks := []func(string) Check{
		checkGoModule,
		checkGoVersion,
		checkProjectLayout,
		checkDIReady,
		checkProtoFiles,
		checkOpenAPIFiles,
		checkWritable,
		checkMageInstalled,
		checkCircularDeps,
		checkCoreDeps,
		checkModuleCount,
		checkGitClean,
	}

	results := make([]Check, len(checks))
	var wg sync.WaitGroup

	for i, checkFn := range checks {
		wg.Add(1)
		go func(idx int, fn func(string) Check) {
			defer wg.Done()
			results[idx] = fn(dir)
		}(i, checkFn)
	}

	wg.Wait()
	return results
}

// Print renders check results to stdout in the standard doctor format.
func Print(checks []Check) {
	for _, c := range checks {
		mark := "✓"
		if c.Status == StatusWarn {
			mark = "!"
		} else if c.Status == StatusFail {
			mark = "✗"
		}
		fmt.Printf("  %s %-20s %s\n", mark, c.Name, c.Detail)
		if c.Hint != "" && c.Status != StatusOK {
			fmt.Printf("      hint: %s\n", c.Hint)
		}
	}
}

// HasFailures returns true if any check failed.
func HasFailures(checks []Check) bool {
	for _, c := range checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// ─── individual checks ────────────────────────────────────────────────────────

func checkGoModule(dir string) Check {
	gomod := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		return Check{
			Name:   "go module",
			Status: StatusFail,
			Detail: "go.mod not found",
			Hint:   "run 'go mod init <module-path>' to initialise a module",
		}
	}
	module := extractModule(string(data))
	if module == "" {
		return Check{
			Name:   "go module",
			Status: StatusWarn,
			Detail: "go.mod found but module name is empty",
		}
	}
	return Check{Name: "go module", Status: StatusOK, Detail: module}
}

func extractModule(gomod string) string {
	re := regexp.MustCompile(`(?m)^module\s+(\S+)`)
	m := re.FindStringSubmatch(gomod)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func checkProjectLayout(dir string) Check {
	simpleDirs := []string{"handler", "service", "repository", "model"}
	dddDirs := []string{
		filepath.Join("cmd", "server"),
		filepath.Join("internal", "domain"),
		filepath.Join("internal", "application"),
		filepath.Join("internal", "infrastructure"),
	}

	simpleCount := countExisting(dir, simpleDirs)
	dddCount := countExisting(dir, dddDirs)

	switch {
	case simpleCount == len(simpleDirs):
		return Check{
			Name:   "project layout",
			Status: StatusOK,
			Detail: "simple  (handler/, service/, repository/, model/)",
		}
	case dddCount >= 2:
		return Check{
			Name:   "project layout",
			Status: StatusOK,
			Detail: "ddd  (cmd/server/, internal/domain/, ...)",
		}
	case simpleCount > 0:
		missing := missingDirs(dir, simpleDirs)
		return Check{
			Name:   "project layout",
			Status: StatusWarn,
			Detail: fmt.Sprintf("partial simple layout — missing: %s", strings.Join(missing, ", ")),
			Hint:   "run 'astractl new <name>' to scaffold a complete project structure",
		}
	default:
		return Check{
			Name:   "project layout",
			Status: StatusWarn,
			Detail: "unknown layout (no standard directories detected)",
			Hint:   "gen commands work from any directory; use --dir to target a specific path",
		}
	}
}

func countExisting(base string, dirs []string) int {
	n := 0
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(base, d)); err == nil {
			n++
		}
	}
	return n
}

func missingDirs(base string, dirs []string) []string {
	var missing []string
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(base, d)); err != nil {
			missing = append(missing, d+"/")
		}
	}
	return missing
}

func checkDIReady(dir string) Check {
	found, err := grepRecursive(dir, `di\.Provide`)
	if err != nil {
		return Check{
			Name:   "di scan ready",
			Status: StatusWarn,
			Detail: "could not scan directory: " + err.Error(),
		}
	}
	if !found {
		return Check{
			Name:   "di scan ready",
			Status: StatusFail,
			Detail: "no di.Provide* calls found",
			Hint:   "add di.Provide[YourType](c, NewYourType) in any .go file, then run 'astractl gen wire --scan'",
		}
	}
	return Check{Name: "di scan ready", Status: StatusOK, Detail: "di.Provide* call(s) found"}
}

func checkProtoFiles(dir string) Check {
	matches, err := filepath.Glob(filepath.Join(dir, "*.proto"))
	if err != nil || len(matches) == 0 {
		return Check{
			Name:   "proto files",
			Status: StatusWarn,
			Detail: "no *.proto files in current directory",
			Hint:   "provide the proto file path explicitly: astractl gen proto path/to/service.proto",
		}
	}
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = filepath.Base(m)
	}
	return Check{
		Name:   "proto files",
		Status: StatusOK,
		Detail: strings.Join(names, ", "),
	}
}

func checkOpenAPIFiles(dir string) Check {
	candidates := []string{
		"openapi.yaml", "openapi.yml", "openapi.json",
		"swagger.yaml", "swagger.yml", "swagger.json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(dir, c)); err == nil {
			return Check{Name: "openapi files", Status: StatusOK, Detail: c}
		}
	}
	return Check{
		Name:   "openapi files",
		Status: StatusWarn,
		Detail: "no openapi.yaml / swagger.yaml found",
		Hint:   "provide the spec path explicitly: astractl gen openapi path/to/openapi.yaml",
	}
}

func checkWritable(dir string) Check {
	f, err := os.CreateTemp(dir, ".astractl_probe_*")
	if err != nil {
		return Check{
			Name:   "writable dir",
			Status: StatusFail,
			Detail: fmt.Sprintf("%s is not writable: %v", dir, err),
			Hint:   "check directory permissions or use --dir to target a writable path",
		}
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return Check{
			Name:   "writable dir",
			Status: StatusWarn,
			Detail: fmt.Sprintf("%s is writable but probe file was not removed: %v", dir, err),
			Hint:   fmt.Sprintf("manually remove: %s", name),
		}
	}
	return Check{Name: "writable dir", Status: StatusOK, Detail: dir}
}

// grepRecursive searches for pattern in all .go files under root.
func grepRecursive(root, pattern string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	found := false
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if re.MatchString(scanner.Text()) {
				found = true
				return nil
			}
		}
		return nil
	})
	return found, err
}
