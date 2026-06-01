package router

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Visualize returns a human-readable tree representation of all registered routes.
// The output shows the radix tree structure with node types, path patterns, and
// handler counts for debugging and documentation purposes.
//
// Example output:
//
//	GET /
//	  └─ users
//	      ├─ [static] /list (1 handler)
//	      ├─ [:id] (1 handler)
//	      │   └─ /edit (1 handler)
//	      └─ [*rest] (1 handler)
//	POST /
//	  └─ users (1 handler)
//
// This method acquires a read lock and is safe to call concurrently with Handle().
func (r *Router) Visualize() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var buf bytes.Buffer

	// Sort methods for deterministic output
	methods := make([]string, 0, len(r.trees))
	for method := range r.trees {
		methods = append(methods, method)
	}
	sort.Strings(methods)

	for i, method := range methods {
		if i > 0 {
			buf.WriteString("\n")
		}
		root := r.trees[method]
		buf.WriteString(fmt.Sprintf("%s /\n", method))
		visualizeNode(root, &buf, "", true)
	}

	return buf.String()
}

// visualizeNode recursively renders a node and its children with tree-drawing characters.
// prefix accumulates the indentation and tree-drawing characters for nested levels.
// isLast indicates whether this node is the last child of its parent (affects the prefix).
func visualizeNode(n *node, buf *bytes.Buffer, prefix string, isLast bool) {
	if n.path == "/" {
		// Root node: render children directly without drawing the root itself
		renderChildren(n, buf, "")
		return
	}

	// Determine the tree-drawing characters for this node
	var connector, childPrefix string
	if isLast {
		connector = "└─ "
		childPrefix = prefix + "    "
	} else {
		connector = "├─ "
		childPrefix = prefix + "│   "
	}

	// Format the node label with type annotation and handler count
	label := formatNodeLabel(n)
	buf.WriteString(prefix + connector + label + "\n")

	// Render children with updated prefix
	renderChildren(n, buf, childPrefix)
}

// formatNodeLabel returns a formatted string for a node showing its path, type, and handlers.
func formatNodeLabel(n *node) string {
	var typeLabel string
	switch n.nType {
	case staticNode:
		typeLabel = ""
	case paramNode:
		typeLabel = fmt.Sprintf("[:%s] ", n.paramKey)
	case regexNode:
		pattern := extractRegexPattern(n.regex.String())
		typeLabel = fmt.Sprintf("[{%s:%s}] ", n.paramKey, pattern)
	case catchAllNode:
		typeLabel = fmt.Sprintf("[*%s] ", n.paramKey)
	}

	path := n.path
	if n.nType == staticNode {
		path = "/" + path
	}

	if n.handlers != nil {
		return fmt.Sprintf("%s%s (%d handler%s)", typeLabel, path, len(n.handlers), plural(len(n.handlers)))
	}
	return fmt.Sprintf("%s%s", typeLabel, path)
}

// extractRegexPattern strips the anchoring wrapper "^(?:<pattern>)$" added by getOrCompileRegexp.
func extractRegexPattern(anchored string) string {
	// anchored format: "^(?:<pattern>)$"
	if strings.HasPrefix(anchored, "^(?:") && strings.HasSuffix(anchored, ")$") {
		return anchored[4 : len(anchored)-2]
	}
	return anchored
}

// renderChildren renders all child nodes of n in a deterministic order:
// static children (sorted), regex children, param child, catch-all child.
func renderChildren(n *node, buf *bytes.Buffer, prefix string) {
	totalChildren := len(n.children) + len(n.regexChildren)
	if n.param != nil {
		totalChildren++
	}
	if n.catchAll != nil {
		totalChildren++
	}

	childIndex := 0

	// Render static children (sorted by path for deterministic output)
	sortedStatic := make([]*node, len(n.children))
	copy(sortedStatic, n.children)
	sort.Slice(sortedStatic, func(i, j int) bool {
		return sortedStatic[i].path < sortedStatic[j].path
	})
	for _, child := range sortedStatic {
		childIndex++
		visualizeNode(child, buf, prefix, childIndex == totalChildren)
	}

	// Render regex children (sorted by paramKey for deterministic output)
	sortedRegex := make([]*node, len(n.regexChildren))
	copy(sortedRegex, n.regexChildren)
	sort.Slice(sortedRegex, func(i, j int) bool {
		return sortedRegex[i].paramKey < sortedRegex[j].paramKey
	})
	for _, child := range sortedRegex {
		childIndex++
		visualizeNode(child, buf, prefix, childIndex == totalChildren)
	}

	// Render param child
	if n.param != nil {
		childIndex++
		visualizeNode(n.param, buf, prefix, childIndex == totalChildren)
	}

	// Render catch-all child
	if n.catchAll != nil {
		childIndex++
		visualizeNode(n.catchAll, buf, prefix, childIndex == totalChildren)
	}
}

// plural returns "s" if count != 1, otherwise "".
func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
