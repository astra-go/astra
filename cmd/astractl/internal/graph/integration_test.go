// +build integration

package graph_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/astra-go/astra/cmd/astractl/internal/graph"
)

// TestParser_Parse_Integration tests the parser against a real Go project.
// This test requires a valid Go project with go.mod in the test directory.
func TestParser_Parse_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use the current project root (astra monorepo)
	projectRoot := findProjectRoot(t)

	parser := graph.NewParser(30 * time.Second)
	result, err := parser.Parse(projectRoot)

	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if result == nil {
		t.Fatal("Parse() returned nil graph")
	}

	if len(result.Nodes) == 0 {
		t.Error("Parse() returned graph with no nodes")
	}

	if len(result.Edges) == 0 {
		t.Error("Parse() returned graph with no edges")
	}

	t.Logf("Parsed %d nodes and %d edges", len(result.Nodes), len(result.Edges))

	// Verify some expected nodes exist in the astra project
	// Note: go list -deps includes dependencies, not just packages in the current module
	expectedNodes := []string{
		"github.com/astra-go/astra/cmd/astractl",
		"github.com/astra-go/astra/cmd/astractl/internal/graph",
	}

	for _, expected := range expectedNodes {
		if _, ok := result.GetNode(expected); !ok {
			t.Errorf("expected node %q not found in graph", expected)
		}
	}
}

// TestCacheManager_Integration tests the full cache lifecycle.
func TestCacheManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	projectRoot := findProjectRoot(t)
	goModPath := filepath.Join(projectRoot, "go.mod")

	// Create temporary cache directory
	tmpDir := t.TempDir()
	cacheMgr := graph.NewCacheManager(tmpDir)

	// First load should return nil (cache doesn't exist)
	cached, err := cacheMgr.Load(goModPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cached != nil {
		t.Error("Load() should return nil for non-existent cache")
	}

	// Parse the project - use a subdirectory that's stable
	parser := graph.NewParser(30 * time.Second)
	// Parse from the graph package itself, not from temp directory
	parseDir := filepath.Join(projectRoot, "cmd", "astractl", "internal", "graph")
	depGraph, err := parser.Parse(parseDir)
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Save to cache
	if err := cacheMgr.Save(depGraph, goModPath); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify cache file exists
	cachePath := cacheMgr.GetCachePath()
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Errorf("cache file not created at %s", cachePath)
	}

	// Load from cache
	loaded, err := cacheMgr.Load(goModPath)
	if err != nil {
		t.Fatalf("Load() from cache failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load() returned nil for existing valid cache")
	}

	// Verify loaded graph matches original
	if len(loaded.Nodes) != len(depGraph.Nodes) {
		t.Errorf("loaded graph has %d nodes, expected %d", len(loaded.Nodes), len(depGraph.Nodes))
	}
	if len(loaded.Edges) != len(depGraph.Edges) {
		t.Errorf("loaded graph has %d edges, expected %d", len(loaded.Edges), len(depGraph.Edges))
	}

	// Clear cache
	if err := cacheMgr.Clear(); err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	// Verify cache file is removed
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("cache file still exists after Clear()")
	}
}

// TestRenderer_Integration tests rendering to actual files.
func TestRenderer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a simple test graph
	g := graph.NewGraph()
	g.AddNode(&graph.Node{
		ImportPath: "github.com/example/pkg1",
		Module:     "github.com/example",
		Standard:   false,
		Deps:       []string{"github.com/example/pkg2", "fmt"},
	})
	g.AddNode(&graph.Node{
		ImportPath: "github.com/example/pkg2",
		Module:     "github.com/example",
		Standard:   false,
		Deps:       []string{"strings"},
	})
	g.AddNode(&graph.Node{
		ImportPath: "fmt",
		Standard:   true,
	})
	g.AddNode(&graph.Node{
		ImportPath: "strings",
		Standard:   true,
	})
	g.AddEdge("github.com/example/pkg1", "github.com/example/pkg2")
	g.AddEdge("github.com/example/pkg1", "fmt")
	g.AddEdge("github.com/example/pkg2", "strings")

	renderer := graph.NewRenderer()
	tmpDir := t.TempDir()

	tests := []struct {
		name   string
		format graph.RenderFormat
		opts   graph.RenderOptions
	}{
		{
			name:   "DOT format",
			format: graph.FormatDOT,
			opts: graph.RenderOptions{
				Format:        graph.FormatDOT,
				OutputPath:    filepath.Join(tmpDir, "test.dot"),
				IncludeStdlib: true,
			},
		},
		{
			name:   "DOT format without stdlib",
			format: graph.FormatDOT,
			opts: graph.RenderOptions{
				Format:        graph.FormatDOT,
				OutputPath:    filepath.Join(tmpDir, "test-no-stdlib.dot"),
				IncludeStdlib: false,
			},
		},
		{
			name:   "DOT format with filter",
			format: graph.FormatDOT,
			opts: graph.RenderOptions{
				Format:        graph.FormatDOT,
				OutputPath:    filepath.Join(tmpDir, "test-filtered.dot"),
				IncludeStdlib: false,
				FilterPrefix:  "github.com/example/pkg1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renderer.Render(g, tt.opts)
			if err != nil {
				t.Fatalf("Render() failed: %v", err)
			}

			// Verify file exists and has content
			content, err := os.ReadFile(tt.opts.OutputPath)
			if err != nil {
				t.Fatalf("failed to read output file: %v", err)
			}

			if len(content) == 0 {
				t.Error("output file is empty")
			}

			// Verify it's valid DOT format
			if !containsSubstring(string(content), "digraph dependencies") {
				t.Error("output does not contain digraph declaration")
			}

			t.Logf("Generated %d bytes to %s", len(content), tt.opts.OutputPath)
		})
	}
}

// TestParser_Parse_Timeout tests that the parser respects timeout.
func TestParser_Parse_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use very short timeout to trigger timeout error
	parser := graph.NewParser(1 * time.Nanosecond)
	projectRoot := findProjectRoot(t)

	_, err := parser.Parse(projectRoot)

	if err == nil {
		t.Error("Parse() should return timeout error with 1ns timeout")
	}

	if err != nil && !containsSubstring(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// TestCachedGraph_IsValid_Integration tests cache invalidation scenarios.
func TestCachedGraph_IsValid_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	projectRoot := findProjectRoot(t)
	goModPath := filepath.Join(projectRoot, "go.mod")

	// Compute current hash
	hash, err := graph.ComputeGoModHash(goModPath)
	if err != nil {
		t.Fatalf("ComputeGoModHash() failed: %v", err)
	}

	g := graph.NewGraph()
	cached := graph.NewCachedGraph(g, hash)

	// Should be valid with matching hash
	if !cached.IsValid(hash) {
		t.Error("cache should be valid with matching hash")
	}

	// Should be invalid with different hash
	if cached.IsValid("different-hash") {
		t.Error("cache should be invalid with different hash")
	}

	// Should be invalid after TTL expiration
	cached.TTL = 1 // 1 second
	cached.Timestamp = time.Now().Add(-2 * time.Second)
	if cached.IsValid(hash) {
		t.Error("cache should be invalid after TTL expiration")
	}
}

// Helper functions

func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Start from current directory and walk up to find go.work
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	// Fallback: use current directory if go.work not found
	wd, _ := os.Getwd()
	return wd
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
