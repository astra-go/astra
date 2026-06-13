package graph

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Node represents a Go module or package in the dependency graph.
type Node struct {
	ImportPath string   `json:"import_path"`
	Module     string   `json:"module,omitempty"`
	Standard   bool     `json:"standard,omitempty"`
	Deps       []string `json:"deps,omitempty"`
}

// Edge represents a dependency relationship between two nodes.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Graph represents a complete dependency graph.
type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
	Edges []Edge           `json:"edges"`
}

// NewGraph creates a new empty dependency graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		Edges: make([]Edge, 0),
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node *Node) {
	if node == nil || node.ImportPath == "" {
		return
	}
	g.Nodes[node.ImportPath] = node
}

// AddEdge adds an edge to the graph.
func (g *Graph) AddEdge(from, to string) {
	if from == "" || to == "" {
		return
	}
	g.Edges = append(g.Edges, Edge{From: from, To: to})
}

// GetNode retrieves a node by import path.
func (g *Graph) GetNode(importPath string) (*Node, bool) {
	node, ok := g.Nodes[importPath]
	return node, ok
}

// CachedGraph represents a cached dependency graph with metadata.
type CachedGraph struct {
	Graph     *Graph    `json:"graph"`
	Hash      string    `json:"hash"`       // SHA256 hash of go.mod
	Timestamp time.Time `json:"timestamp"`  // Cache creation time
	TTL       int64     `json:"ttl"`        // Time-to-live in seconds (default: 3600)
}

// NewCachedGraph creates a new cached graph.
func NewCachedGraph(graph *Graph, hash string) *CachedGraph {
	return &CachedGraph{
		Graph:     graph,
		Hash:      hash,
		Timestamp: time.Now(),
		TTL:       3600, // 1 hour
	}
}

// IsValid checks if the cached graph is still valid.
// Returns false if:
// - Cache has expired (timestamp + TTL < now)
// - go.mod hash has changed
func (c *CachedGraph) IsValid(currentHash string) bool {
	if c == nil || c.Graph == nil {
		return false
	}

	// Check hash mismatch
	if c.Hash != currentHash {
		return false
	}

	// Check expiration
	expiresAt := c.Timestamp.Add(time.Duration(c.TTL) * time.Second)
	if time.Now().After(expiresAt) {
		return false
	}

	return true
}

// SaveToFile serializes the cached graph to a JSON file.
func (c *CachedGraph) SaveToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("failed to encode cache: %w", err)
	}

	return nil
}

// LoadCachedGraphFromFile loads a cached graph from a JSON file.
func LoadCachedGraphFromFile(path string) (*CachedGraph, error) {
	file, err := os.Open(path) // 
	if err != nil {
		return nil, fmt.Errorf("failed to open cache file: %w", err)
	}
	defer file.Close()

	var cached CachedGraph
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cached); err != nil {
		return nil, fmt.Errorf("failed to decode cache: %w", err)
	}

	return &cached, nil
}

// ComputeGoModHash computes SHA256 hash of go.mod file.
func ComputeGoModHash(goModPath string) (string, error) {
	file, err := os.Open(goModPath) // 
	if err != nil {
		return "", fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
