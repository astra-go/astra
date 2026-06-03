package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckGoVersion(t *testing.T) {
	result := checkGoVersion(".")

	if result.Name != "go version" {
		t.Errorf("expected name 'go version', got %q", result.Name)
	}

	// Should succeed or warn, not fail (unless Go is completely missing)
	if result.Status != StatusOK && result.Status != StatusWarn {
		t.Errorf("unexpected status: %s", result.Status)
	}

	t.Logf("Go version check: %s - %s", result.Status, result.Detail)
}

func TestCheckMageInstalled(t *testing.T) {
	result := checkMageInstalled(".")

	if result.Name != "mage installed" {
		t.Errorf("expected name 'mage installed', got %q", result.Name)
	}

	// Should be OK or Warn, never Fail
	if result.Status == StatusFail {
		t.Errorf("checkMageInstalled should not fail, got: %s", result.Status)
	}

	t.Logf("Mage check: %s - %s", result.Status, result.Detail)
}

func TestCheckCircularDeps(t *testing.T) {
	result := checkCircularDeps(".")

	if result.Name != "circular deps" {
		t.Errorf("expected name 'circular deps', got %q", result.Name)
	}

	// Should succeed or warn, not fail
	if result.Status == StatusFail {
		t.Errorf("checkCircularDeps should not fail, got: %s", result.Status)
	}

	t.Logf("Circular deps check: %s - %s", result.Status, result.Detail)
}

func TestCheckCoreDeps(t *testing.T) {
	result := checkCoreDeps(".")

	if result.Name != "core deps (ADR-001)" {
		t.Errorf("expected name 'core deps (ADR-001)', got %q", result.Name)
	}

	// Should always be OK (either not applicable or delegated to mage)
	if result.Status == StatusFail {
		t.Errorf("checkCoreDeps should not fail, got: %s", result.Status)
	}

	t.Logf("Core deps check: %s - %s", result.Status, result.Detail)
}

func TestCheckModuleCount(t *testing.T) {
	result := checkModuleCount(".")

	if result.Name != "module count (ADR-005)" {
		t.Errorf("expected name 'module count (ADR-005)', got %q", result.Name)
	}

	// Should be OK or Warn
	if result.Status == StatusFail {
		t.Errorf("checkModuleCount should not fail, got: %s", result.Status)
	}

	t.Logf("Module count check: %s - %s", result.Status, result.Detail)
}

func TestCheckGitClean(t *testing.T) {
	result := checkGitClean(".")

	if result.Name != "git working tree" {
		t.Errorf("expected name 'git working tree', got %q", result.Name)
	}

	// Should be OK or Warn, never Fail
	if result.Status == StatusFail {
		t.Errorf("checkGitClean should not fail, got: %s", result.Status)
	}

	t.Logf("Git clean check: %s - %s", result.Status, result.Detail)
}

func TestCheckModuleCount_WithGoWork(t *testing.T) {
	// Create temporary directory with mock go.work
	tmpDir := t.TempDir()

	goWork := `go 1.25

use (
	./core
	./netengine
	./examples/hello
	./cmd/astractl
	./examples/mq
)
`
	err := os.WriteFile(filepath.Join(tmpDir, "go.work"), []byte(goWork), 0644)
	if err != nil {
		t.Fatalf("failed to create test go.work: %v", err)
	}

	result := checkModuleCount(tmpDir)

	if result.Status != StatusOK {
		t.Errorf("expected status OK for 5 modules, got %s", result.Status)
	}

	if !strings.Contains(result.Detail, "5 modules") {
		t.Errorf("expected detail to contain '5 modules', got %q", result.Detail)
	}
}

func TestCheckModuleCount_ExceedLimit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.work with 45 modules (exceeds 40 limit)
	var sb strings.Builder
	sb.WriteString("go 1.25\n\nuse (\n")
	for i := 1; i <= 45; i++ {
		sb.WriteString("	./module")
		sb.WriteString(string(rune('0' + (i % 10))))
		sb.WriteString("\n")
	}
	sb.WriteString(")\n")

	err := os.WriteFile(filepath.Join(tmpDir, "go.work"), []byte(sb.String()), 0644)
	if err != nil {
		t.Fatalf("failed to create test go.work: %v", err)
	}

	result := checkModuleCount(tmpDir)

	if result.Status != StatusWarn {
		t.Errorf("expected status Warn for 45 modules, got %s", result.Status)
	}

	if !strings.Contains(result.Detail, "45 modules") {
		t.Errorf("expected detail to contain '45 modules', got %q", result.Detail)
	}
}

func TestRun_ParallelExecution(t *testing.T) {
	// Test that Run executes checks in parallel
	results := Run(".")

	// Should have 12 checks now (6 original + 6 new)
	expectedCount := 12
	if len(results) != expectedCount {
		t.Errorf("expected %d checks, got %d", expectedCount, len(results))
	}

	// Verify all check names are present
	expectedNames := []string{
		"go module",
		"go version",
		"project layout",
		"di scan ready",
		"proto files",
		"openapi files",
		"writable dir",
		"mage installed",
		"circular deps",
		"core deps (ADR-001)",
		"module count (ADR-005)",
		"git working tree",
	}

	foundNames := make(map[string]bool)
	for _, check := range results {
		foundNames[check.Name] = true
	}

	for _, name := range expectedNames {
		if !foundNames[name] {
			t.Errorf("check %q not found in results", name)
		}
	}
}

func TestPrint(t *testing.T) {
	checks := []Check{
		{Name: "test ok", Status: StatusOK, Detail: "all good"},
		{Name: "test warn", Status: StatusWarn, Detail: "warning", Hint: "fix this"},
		{Name: "test fail", Status: StatusFail, Detail: "failed", Hint: "fix that"},
	}

	// Print should not panic
	Print(checks)
}

func TestHasFailures(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   bool
	}{
		{
			name: "no failures",
			checks: []Check{
				{Status: StatusOK},
				{Status: StatusWarn},
			},
			want: false,
		},
		{
			name: "has failure",
			checks: []Check{
				{Status: StatusOK},
				{Status: StatusFail},
			},
			want: true,
		},
		{
			name: "multiple failures",
			checks: []Check{
				{Status: StatusFail},
				{Status: StatusFail},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasFailures(tt.checks)
			if got != tt.want {
				t.Errorf("HasFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}
