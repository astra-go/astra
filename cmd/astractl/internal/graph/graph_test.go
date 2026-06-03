package graph

import (
	"testing"
	"time"
)

func TestGraph_AddNode(t *testing.T) {
	g := NewGraph()

	node := &Node{
		ImportPath: "github.com/example/pkg",
		Module:     "github.com/example",
		Standard:   false,
		Deps:       []string{"fmt", "strings"},
	}

	g.AddNode(node)

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}

	retrieved, ok := g.GetNode("github.com/example/pkg")
	if !ok {
		t.Error("node not found in graph")
	}

	if retrieved.ImportPath != node.ImportPath {
		t.Errorf("expected import path %s, got %s", node.ImportPath, retrieved.ImportPath)
	}
}

func TestGraph_AddNode_Nil(t *testing.T) {
	g := NewGraph()
	g.AddNode(nil)

	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes after adding nil, got %d", len(g.Nodes))
	}
}

func TestGraph_AddNode_EmptyImportPath(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{ImportPath: ""})

	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes after adding empty import path, got %d", len(g.Nodes))
	}
}

func TestGraph_AddEdge(t *testing.T) {
	g := NewGraph()
	g.AddEdge("pkg1", "pkg2")
	g.AddEdge("pkg2", "pkg3")

	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(g.Edges))
	}

	if g.Edges[0].From != "pkg1" || g.Edges[0].To != "pkg2" {
		t.Errorf("unexpected edge: %v", g.Edges[0])
	}
}

func TestGraph_AddEdge_EmptyNodes(t *testing.T) {
	g := NewGraph()
	g.AddEdge("", "pkg2")
	g.AddEdge("pkg1", "")

	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges after adding empty nodes, got %d", len(g.Edges))
	}
}

func TestCachedGraph_IsValid_ValidCache(t *testing.T) {
	graph := NewGraph()
	hash := "abc123"
	cached := NewCachedGraph(graph, hash)

	if !cached.IsValid(hash) {
		t.Error("expected cache to be valid with matching hash")
	}
}

func TestCachedGraph_IsValid_HashMismatch(t *testing.T) {
	graph := NewGraph()
	cached := NewCachedGraph(graph, "abc123")

	if cached.IsValid("xyz789") {
		t.Error("expected cache to be invalid with mismatched hash")
	}
}

func TestCachedGraph_IsValid_Expired(t *testing.T) {
	graph := NewGraph()
	cached := NewCachedGraph(graph, "abc123")
	cached.TTL = 1 // 1 second
	cached.Timestamp = time.Now().Add(-2 * time.Second)

	if cached.IsValid("abc123") {
		t.Error("expected cache to be invalid after expiration")
	}
}

func TestCachedGraph_IsValid_NilGraph(t *testing.T) {
	cached := &CachedGraph{
		Graph:     nil,
		Hash:      "abc123",
		Timestamp: time.Now(),
		TTL:       3600,
	}

	if cached.IsValid("abc123") {
		t.Error("expected cache to be invalid with nil graph")
	}
}

func TestNewParser(t *testing.T) {
	timeout := 30 * time.Second
	parser := NewParser(timeout)

	if parser.timeout != timeout {
		t.Errorf("expected timeout %v, got %v", timeout, parser.timeout)
	}
}

func TestRenderer_GenerateDOT_EmptyGraph(t *testing.T) {
	renderer := NewRenderer()
	graph := NewGraph()

	opts := RenderOptions{
		Format:        FormatDOT,
		IncludeStdlib: false,
	}

	dot := renderer.generateDOT(graph, opts)

	if dot == "" {
		t.Error("expected non-empty DOT output")
	}

	// Should contain digraph declaration
	if !containsString(dot, "digraph dependencies") {
		t.Error("DOT output missing digraph declaration")
	}
}

func TestRenderer_GenerateDOT_WithNodes(t *testing.T) {
	renderer := NewRenderer()
	graph := NewGraph()

	graph.AddNode(&Node{
		ImportPath: "github.com/example/pkg",
		Module:     "github.com/example",
		Standard:   false,
	})

	graph.AddNode(&Node{
		ImportPath: "fmt",
		Standard:   true,
	})

	graph.AddEdge("github.com/example/pkg", "fmt")

	opts := RenderOptions{
		Format:        FormatDOT,
		IncludeStdlib: true,
	}

	dot := renderer.generateDOT(graph, opts)

	if !containsString(dot, "github.com/example/pkg") {
		t.Error("DOT output missing node")
	}

	if !containsString(dot, "fmt") {
		t.Error("DOT output missing standard library node")
	}

	if !containsString(dot, "->") {
		t.Error("DOT output missing edge")
	}
}

func TestRenderer_GenerateDOT_FilterStdlib(t *testing.T) {
	renderer := NewRenderer()
	graph := NewGraph()

	graph.AddNode(&Node{
		ImportPath: "github.com/example/pkg",
		Standard:   false,
	})

	graph.AddNode(&Node{
		ImportPath: "fmt",
		Standard:   true,
	})

	opts := RenderOptions{
		Format:        FormatDOT,
		IncludeStdlib: false,
	}

	dot := renderer.generateDOT(graph, opts)

	if !containsString(dot, "github.com/example/pkg") {
		t.Error("DOT output missing non-standard node")
	}

	if containsString(dot, "fmt") {
		t.Error("DOT output should not include standard library node")
	}
}

func TestRenderer_GenerateDOT_FilterPrefix(t *testing.T) {
	renderer := NewRenderer()
	graph := NewGraph()

	graph.AddNode(&Node{
		ImportPath: "github.com/astra-go/astra/core",
		Standard:   false,
	})

	graph.AddNode(&Node{
		ImportPath: "github.com/other/pkg",
		Standard:   false,
	})

	opts := RenderOptions{
		Format:       FormatDOT,
		FilterPrefix: "github.com/astra-go/astra",
	}

	dot := renderer.generateDOT(graph, opts)

	if !containsString(dot, "github.com/astra-go/astra/core") {
		t.Error("DOT output missing filtered node")
	}

	if containsString(dot, "github.com/other/pkg") {
		t.Error("DOT output should not include node outside filter prefix")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
