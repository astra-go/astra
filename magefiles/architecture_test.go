//go:build mage

package main

import (
	"testing"
)

// TestMatchForbiddenRule tests pattern matching logic against various dependencies
func TestMatchForbiddenRule(t *testing.T) {
	rules := []ArchRule{
		{
			Pattern:    "gorm.io/**",
			Reason:     "ORM libraries must be in orm/ sub-module",
			FixHint:    "Use contract.Repository[T] interface",
			ADR:        "docs/adr/ADR-001-core-dependency-boundary.md",
			Exceptions: []string{},
		},
		{
			Pattern:    "github.com/redis/go-redis/**",
			Reason:     "Redis client must be in cache/ sub-module",
			FixHint:    "Import github.com/astra-go/astra/cache",
			ADR:        "docs/adr/ADR-001-core-dependency-boundary.md",
			Exceptions: []string{},
		},
		{
			Pattern: "go.opentelemetry.io/otel/**",
			Reason:  "OpenTelemetry libs must be in otel/ sub-module",
			FixHint: "Import github.com/astra-go/astra/otel",
			ADR:     "docs/adr/ADR-001-core-dependency-boundary.md",
			Exceptions: []string{
				"go.opentelemetry.io/otel/trace/noop",
			},
		},
	}

	tests := []struct {
		name      string
		dep       string
		wantMatch bool
	}{
		{"gorm base package", "gorm.io/gorm", true},
		{"gorm driver", "gorm.io/driver/mysql", true},
		{"redis client", "github.com/redis/go-redis/v9", true},
		{"otel sdk", "go.opentelemetry.io/otel/sdk", true},
		{"otel noop (exception)", "go.opentelemetry.io/otel/trace/noop", false},
		{"golang official pkg", "golang.org/x/net/http2", false},
		{"json library", "github.com/goccy/go-json", false},
		{"stdlib", "net/http", false},
		{"validator", "github.com/go-playground/validator/v10", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := matchForbiddenRule(tt.dep, rules) != nil
			if matched != tt.wantMatch {
				t.Errorf("matchForbiddenRule(%q) = %v, want %v", tt.dep, matched, tt.wantMatch)
			}
		})
	}
}

// TestGlobToRegex tests glob pattern to regex conversion
func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		testString string
		wantMatch  bool
	}{
		{"double star matches nested path", "gorm.io/**", "gorm.io/driver/mysql", true},
		{"double star matches single level", "gorm.io/**", "gorm.io/gorm", true},
		{"double star no match different base", "gorm.io/**", "github.com/gorm", false},
		{"single star matches segment", "github.com/*/kafka-go", "github.com/segmentio/kafka-go", true},
		{"single star no match multiple segments", "github.com/*/kafka-go", "github.com/segmentio/libs/kafka-go", false},
		{"exact match", "github.com/redis/go-redis/v9", "github.com/redis/go-redis/v9", true},
		{"exact no match", "github.com/redis/go-redis/v9", "github.com/redis/go-redis/v8", false},
		{"pattern with dots", "go.etcd.io/**", "go.etcd.io/etcd/client/v3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regex := globToRegex(tt.pattern)
			matched := regex.MatchString(tt.testString)
			if matched != tt.wantMatch {
				t.Errorf("globToRegex(%q).MatchString(%q) = %v, want %v",
					tt.pattern, tt.testString, matched, tt.wantMatch)
			}
		})
	}
}

// TestIsException tests exception list handling
func TestIsException(t *testing.T) {
	exceptions := []string{
		"go.opentelemetry.io/otel/trace/noop",
		"github.com/special/allowed",
	}

	tests := []struct {
		name          string
		dep           string
		wantException bool
	}{
		{"exact match exception", "go.opentelemetry.io/otel/trace/noop", true},
		{"another exception", "github.com/special/allowed", true},
		{"not in exception list", "go.opentelemetry.io/otel/sdk", false},
		{"empty dep", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isException(tt.dep, exceptions)
			if result != tt.wantException {
				t.Errorf("isException(%q) = %v, want %v", tt.dep, result, tt.wantException)
			}
		})
	}
}

// TestDetectCycles tests circular dependency detection
func TestDetectCycles(t *testing.T) {
	tests := []struct {
		name       string
		graph      map[string][]string
		wantCycles bool
	}{
		{
			name: "simple cycle A->B->C->A",
			graph: map[string][]string{
				"A": {"B"},
				"B": {"C"},
				"C": {"A"},
			},
			wantCycles: true,
		},
		{
			name: "self loop",
			graph: map[string][]string{
				"A": {"A"},
			},
			wantCycles: true,
		},
		{
			name: "no cycle - linear",
			graph: map[string][]string{
				"A": {"B"},
				"B": {"C"},
				"C": {},
			},
			wantCycles: false,
		},
		{
			name: "no cycle - diamond",
			graph: map[string][]string{
				"A": {"B", "C"},
				"B": {"D"},
				"C": {"D"},
				"D": {},
			},
			wantCycles: false,
		},
		{
			name: "complex cycle",
			graph: map[string][]string{
				"A": {"B"},
				"B": {"C", "D"},
				"C": {"E"},
				"D": {"E"},
				"E": {"B"},
			},
			wantCycles: true,
		},
		{
			name:       "empty graph",
			graph:      map[string][]string{},
			wantCycles: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycles := detectCycles(tt.graph)
			hasCycles := len(cycles) > 0
			if hasCycles != tt.wantCycles {
				t.Errorf("detectCycles() found %d cycles, want cycles: %v", len(cycles), tt.wantCycles)
			}
		})
	}
}

// TestLoadArchRules tests YAML configuration loading
func TestLoadArchRules(t *testing.T) {
	cfg, err := loadArchRules()
	if err != nil {
		t.Fatalf("loadArchRules() failed: %v", err)
	}

	if len(cfg.CoreForbiddenDeps) == 0 {
		t.Error("expected at least one forbidden dependency rule")
	}

	// Check for gorm.io rule
	var foundGorm bool
	for _, rule := range cfg.CoreForbiddenDeps {
		if rule.Pattern == "gorm.io/**" {
			foundGorm = true
			if rule.Reason == "" {
				t.Error("gorm rule should have a reason")
			}
			if rule.FixHint == "" {
				t.Error("gorm rule should have a fix hint")
			}
			if rule.ADR == "" {
				t.Error("gorm rule should reference an ADR")
			}
		}
	}
	if !foundGorm {
		t.Error("expected to find gorm.io/** rule in configuration")
	}

	// Check for otel rule with exceptions
	var foundOtelWithExceptions bool
	for _, rule := range cfg.CoreForbiddenDeps {
		if rule.Pattern == "go.opentelemetry.io/otel/**" {
			if len(rule.Exceptions) > 0 {
				foundOtelWithExceptions = true
			}
		}
	}
	if !foundOtelWithExceptions {
		t.Error("expected otel rule to have exceptions configured")
	}

	if cfg.MaxDepDepth == 0 {
		t.Error("expected MaxDepDepth to be configured")
	}
}
