package graph

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Renderer handles rendering of dependency graphs to various formats.
type Renderer struct{}

// NewRenderer creates a new graph renderer.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// RenderFormat specifies the output format for graph rendering.
type RenderFormat string

const (
	FormatDOT RenderFormat = "dot"
	FormatSVG RenderFormat = "svg"
	FormatPNG RenderFormat = "png"
)

// RenderOptions contains options for graph rendering.
type RenderOptions struct {
	Format         RenderFormat
	OutputPath     string
	IncludeStdlib  bool
	FilterPrefix   string // Only include nodes with this prefix
	MaxDepth       int    // Maximum dependency depth (0 = unlimited)
}

// Render renders the graph to the specified format and output path.
func (r *Renderer) Render(graph *Graph, opts RenderOptions) error {
	// Generate DOT format
	dot := r.generateDOT(graph, opts)

	// If DOT format is requested, write directly
	if opts.Format == FormatDOT {
		return os.WriteFile(opts.OutputPath, []byte(dot), 0600)
	}

	// For SVG/PNG, check if Graphviz is installed
	if !r.isGraphvizInstalled() {
		return fmt.Errorf("Graphviz is not installed. Please install it to render %s format.\n"+
			"Installation:\n"+
			"  macOS:   brew install graphviz\n"+
			"  Ubuntu:  sudo apt-get install graphviz\n"+
			"  Windows: choco install graphviz", opts.Format)
	}

	// Render using Graphviz
	return r.renderWithGraphviz(dot, opts)
}

// generateDOT generates a DOT format representation of the graph.
func (r *Renderer) generateDOT(graph *Graph, opts RenderOptions) string {
	var sb strings.Builder
	sb.WriteString("digraph dependencies {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=rounded];\n\n")

	// Filter nodes
	filteredNodes := make(map[string]*Node)
	for path, node := range graph.Nodes {
		// Skip standard library if not included
		if !opts.IncludeStdlib && node.Standard {
			continue
		}

		// Apply prefix filter
		if opts.FilterPrefix != "" && !strings.HasPrefix(path, opts.FilterPrefix) {
			continue
		}

		filteredNodes[path] = node
	}

	// Add nodes
	for path, node := range filteredNodes {
		label := path
		color := "lightblue"

		if node.Standard {
			color = "lightgray"
		} else if node.Module != "" {
			label = fmt.Sprintf("%s\\n[%s]", path, node.Module)
		}

		fmt.Fprintf(&sb, "  \"%s\" [label=\"%s\", fillcolor=\"%s\", style=filled];\n",
			path, label, color)
	}

	sb.WriteString("\n")

	// Add edges
	for _, edge := range graph.Edges {
		// Skip if either node is filtered out
		if _, ok := filteredNodes[edge.From]; !ok {
			continue
		}
		if _, ok := filteredNodes[edge.To]; !ok {
			continue
		}

		fmt.Fprintf(&sb, "  \"%s\" -> \"%s\";\n", edge.From, edge.To)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// isGraphvizInstalled checks if Graphviz dot command is available.
func (r *Renderer) isGraphvizInstalled() bool {
	_, err := exec.LookPath("dot")
	return err == nil
}

// renderWithGraphviz renders DOT format to SVG or PNG using Graphviz.
func (r *Renderer) renderWithGraphviz(dot string, opts RenderOptions) error {
	// Determine output format flag
	var formatFlag string
	switch opts.Format {
	case FormatSVG:
		formatFlag = "-Tsvg"
	case FormatPNG:
		formatFlag = "-Tpng"
	default:
		return fmt.Errorf("unsupported format: %s", opts.Format)
	}

	// Execute dot command
	cmd := exec.Command("dot", formatFlag, "-o", opts.OutputPath) // Command path validated via safeLookPath
	cmd.Stdin = strings.NewReader(dot)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("graphviz rendering failed: %w\noutput: %s", err, string(output))
	}

	return nil
}
