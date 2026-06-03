package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Parser handles parsing of Go module dependency graphs.
type Parser struct {
	timeout time.Duration
}

// NewParser creates a new parser with the specified timeout.
func NewParser(timeout time.Duration) *Parser {
	return &Parser{
		timeout: timeout,
	}
}

// goListOutput represents the JSON output from `go list -json -deps`.
type goListOutput struct {
	ImportPath string   `json:"ImportPath"`
	Module     *struct {
		Path string `json:"Path"`
	} `json:"Module,omitempty"`
	Standard bool     `json:"Standard"`
	Deps     []string `json:"Deps,omitempty"`
}

// Parse executes `go list -json -deps ./...` and builds a dependency graph.
// Returns an error if the command times out or fails.
func (p *Parser) Parse(dir string) (*Graph, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "list", "-json", "-deps", "./...")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("go list command timed out after %v", p.timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list failed: %w\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("go list failed: %w", err)
	}

	return p.parseOutput(output)
}

// parseOutput parses the JSON output from `go list -json -deps`.
func (p *Parser) parseOutput(output []byte) (*Graph, error) {
	graph := NewGraph()
	br := bytesReader(output)
	decoder := json.NewDecoder(&br)

	for decoder.More() {
		var pkg goListOutput
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("failed to decode go list output: %w", err)
		}

		// Create node
		node := &Node{
			ImportPath: pkg.ImportPath,
			Standard:   pkg.Standard,
			Deps:       pkg.Deps,
		}
		if pkg.Module != nil {
			node.Module = pkg.Module.Path
		}

		graph.AddNode(node)

		// Create edges
		for _, dep := range pkg.Deps {
			graph.AddEdge(pkg.ImportPath, dep)
		}
	}

	return graph, nil
}

// bytesReader wraps a byte slice to implement io.Reader.
type bytesReader []byte

func (b *bytesReader) Read(p []byte) (n int, err error) {
	if len(*b) == 0 {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, *b)
	*b = (*b)[n:]
	return n, nil
}
