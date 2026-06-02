//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ArchRule represents a single architecture constraint rule.
type ArchRule struct {
	Pattern    string   `yaml:"pattern"`
	Reason     string   `yaml:"reason"`
	FixHint    string   `yaml:"fix_hint"`
	ADR        string   `yaml:"adr"`
	Exceptions []string `yaml:"exceptions"`
}

// ArchConfig holds the complete architecture rules configuration.
type ArchConfig struct {
	CoreForbiddenDeps []ArchRule `yaml:"core_forbidden_deps"`
	MaxDepDepth       int        `yaml:"max_dep_depth"`
}

// Violation represents a detected architecture rule violation.
type Violation struct {
	Dependency string
	Rule       *ArchRule
}

// CheckCoreDeps checks if the core module depends on any forbidden packages.
// Returns nil if no violations found, error otherwise.
func CheckCoreDeps() error {
	fmt.Println("🔍 Checking core module dependency boundary (ADR-001)...")

	// 1. Load architecture rules
	rules, err := loadArchRules()
	if err != nil {
		return fmt.Errorf("failed to load architecture rules: %w", err)
	}

	// 2. Get transitive dependencies of core module
	deps, err := getTransitiveDeps("github.com/astra-go/astra")
	if err != nil {
		return fmt.Errorf("failed to get dependencies: %w", err)
	}

	// 3. Check for violations
	violations := []Violation{}
	for _, dep := range deps {
		if rule := matchForbiddenRule(dep, rules.CoreForbiddenDeps); rule != nil {
			violations = append(violations, Violation{
				Dependency: dep,
				Rule:       rule,
			})
		}
	}

	// 4. Report results
	if len(violations) > 0 {
		return formatViolations(violations)
	}

	fmt.Println("✅ Core dependency boundary check passed")
	return nil
}

// CheckCircularDeps checks for circular dependencies between sub-modules.
func CheckCircularDeps() error {
	fmt.Println("🔍 Checking for circular dependencies...")

	// Build dependency graph from go mod graph
	graph, err := buildModuleDependencyGraph()
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Detect cycles
	cycles := detectCycles(graph)
	if len(cycles) > 0 {
		return fmt.Errorf("❌ Circular dependencies detected:\n%s", formatCycles(cycles))
	}

	fmt.Println("✅ No circular dependencies detected")
	return nil
}

// loadArchRules loads architecture rules from YAML configuration file.
func loadArchRules() (*ArchConfig, error) {
	configPath := "architecture-rules.yaml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg ArchConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &cfg, nil
}

// getTransitiveDeps returns all transitive dependencies of a module.
func getTransitiveDeps(modulePath string) ([]string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", "-deps", modulePath)
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list failed: %w\nOutput: %s", err, string(out))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")

	// Filter out standard library packages
	var deps []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == modulePath {
			continue
		}
		// Skip standard library (no dots in package path)
		if !strings.Contains(line, ".") {
			continue
		}
		deps = append(deps, line)
	}

	return deps, nil
}

// matchForbiddenRule checks if a dependency matches any forbidden rule.
// Returns the matched rule, or nil if no match.
func matchForbiddenRule(dep string, rules []ArchRule) *ArchRule {
	for i := range rules {
		rule := &rules[i]

		// Convert glob pattern to regex
		pattern := globToRegex(rule.Pattern)
		if !pattern.MatchString(dep) {
			continue
		}

		// Check exceptions
		if isException(dep, rule.Exceptions) {
			continue
		}

		return rule
	}
	return nil
}

// globToRegex converts a glob pattern to a compiled regex.
// Example: "gorm.io/**" → regex matching "gorm.io/.*"
func globToRegex(pattern string) *regexp.Regexp {
	// Escape special regex characters
	s := regexp.QuoteMeta(pattern)

	// Convert glob wildcards to regex
	s = strings.ReplaceAll(s, `\*\*`, ".*")   // ** matches any path
	s = strings.ReplaceAll(s, `\*`, "[^/]*")  // * matches single segment

	return regexp.MustCompile("^" + s + "$")
}

// isException checks if a dependency is in the exceptions list.
func isException(dep string, exceptions []string) bool {
	for _, exc := range exceptions {
		if dep == exc {
			return true
		}
	}
	return false
}

// formatViolations formats a list of violations into a human-readable error.
func formatViolations(violations []Violation) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n❌ Architecture violation detected (%d issues):\n\n", len(violations)))

	for i, v := range violations {
		sb.WriteString(fmt.Sprintf("%d. Core module depends on: %s\n", i+1, v.Dependency))
		sb.WriteString(fmt.Sprintf("   Reason: %s\n", v.Rule.Reason))
		if v.Rule.FixHint != "" {
			sb.WriteString(fmt.Sprintf("   Fix: %s\n", v.Rule.FixHint))
		}
		if v.Rule.ADR != "" {
			sb.WriteString(fmt.Sprintf("   Documentation: %s\n", v.Rule.ADR))
		}
		sb.WriteString("\n")
	}

	return fmt.Errorf("%s", sb.String())
}

// buildModuleDependencyGraph builds a dependency graph from go mod graph output.
func buildModuleDependencyGraph() (map[string][]string, error) {
	cmd := exec.Command("go", "mod", "graph")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go mod graph failed: %w", err)
	}

	graph := make(map[string][]string)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		from, to := parts[0], parts[1]

		// Only track astra-go modules
		if strings.HasPrefix(from, "github.com/astra-go/astra") &&
			strings.HasPrefix(to, "github.com/astra-go/astra") {
			graph[from] = append(graph[from], to)
		}
	}

	return graph, nil
}

// detectCycles detects circular dependencies in a dependency graph.
// Returns a list of cycles found (each cycle is a slice of module names).
func detectCycles(graph map[string][]string) [][]string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cycles [][]string
	var currentPath []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		currentPath = append(currentPath, node)

		for _, neighbor := range graph[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				// Cycle detected - extract the cycle
				cycleStart := -1
				for i, n := range currentPath {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(currentPath)-cycleStart)
					copy(cycle, currentPath[cycleStart:])
					cycles = append(cycles, cycle)
				}
				return true
			}
		}

		recStack[node] = false
		currentPath = currentPath[:len(currentPath)-1]
		return false
	}

	for node := range graph {
		if !visited[node] {
			dfs(node)
		}
	}

	return cycles
}

// formatCycles formats detected cycles into a human-readable string.
func formatCycles(cycles [][]string) string {
	var sb strings.Builder
	for i, cycle := range cycles {
		sb.WriteString(fmt.Sprintf("  Cycle %d: ", i+1))
		for j, mod := range cycle {
			if j > 0 {
				sb.WriteString(" → ")
			}
			// Shorten module names for readability
			short := strings.TrimPrefix(mod, "github.com/astra-go/astra/")
			if short == mod {
				short = "core"
			}
			sb.WriteString(short)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
