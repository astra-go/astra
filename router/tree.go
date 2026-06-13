package router

import (
	"fmt"
	"regexp"
	"strings"
)

// nodeType categorizes each node in the radix tree.
type nodeType uint8

const (
	staticNode   nodeType = iota
	regexNode              // {name:pattern} — regex-constrained param
	paramNode              // :name — unconstrained param
	catchAllNode           // *name — catch-all
)

// childIndexAbsent and childIndexCollision are sentinel values for node.childIndex.
// Using int16 supports up to 32 765 static siblings before the index saturates.
const (
	childIndexAbsent    int16 = -1 // no child has this first byte
	childIndexCollision int16 = -2 // two or more children share this first byte → linear scan
)

// node is a node in the radix tree.
type node struct {
	path          string
	nType         nodeType
	handlers      HandlersChain
	paramKey      string
	regex         *regexp.Regexp // non-nil for regexNode; shared via regexpCache
	fastMatch     fastMatcher    // non-nil when a well-known pattern fast-path is available
	children      []*node        // static children (kept for ordered iteration)
	childIndex    *[256]int16    // first-byte dispatch: childIndex[b] = index into children, or sentinel
	childMap      map[string]*node // non-nil when childIndex has collision buckets; O(1) full-path lookup
	regexChildren []*node        // regex-constrained children (multiple patterns allowed)
	param         *node          // :name child
	catchAll      *node          // *name child
	fullPath      string
}

// newChildIndex allocates and initialises a first-byte dispatch table.
// All 256 entries are set to childIndexAbsent (-1).
func newChildIndex() *[256]int16 {
	idx := new([256]int16)
	for i := range idx {
		idx[i] = childIndexAbsent
	}
	return idx
}

// recordChildIndex updates n.childIndex after a new child was appended to
// n.children at position childPos.  Must be called exactly once per append.
// When two children collide on the same first byte, childMap is populated so
// that matchSegments can resolve the collision in O(1) via a map lookup instead
// of a linear scan over all children.
func recordChildIndex(n *node, childPos int) {
	if len(n.children[childPos].path) == 0 {
		return
	}
	if n.childIndex == nil {
		n.childIndex = newChildIndex()
	}
	b := n.children[childPos].path[0]
	switch n.childIndex[b] {
	case childIndexAbsent:
		n.childIndex[b] = int16(childPos) // childPos 在实际场景中不会超过 32767
	default:
		if n.childIndex[b] != childIndexCollision {
			// First collision for this byte: migrate the previously indexed child
			// into childMap so we have O(1) lookup for both children.
			if n.childMap == nil {
				n.childMap = make(map[string]*node, 4)
			}
			n.childMap[n.children[n.childIndex[b]].path] = n.children[n.childIndex[b]]
			n.childIndex[b] = childIndexCollision
		}
		// Add the newly appended child to childMap.
		if n.childMap == nil {
			n.childMap = make(map[string]*node, 4)
		}
		n.childMap[n.children[childPos].path] = n.children[childPos]
	}
}

func insertNode(root *node, path string, handlers HandlersChain) (overwritten bool) {
	parts := splitPath(path)
	current := root

	// Root path "/"
	if len(parts) == 0 {
		overwritten = root.handlers != nil
		root.handlers = handlers
		root.fullPath = path
		return
	}

	for i, part := range parts {
		// Defensive: skip any empty segment returned by splitPath.
		// splitPath already filters empty strings, but this guard protects
		// against malformed inputs that slip through in future code paths.
		if part == "" {
			panic(fmt.Sprintf("astra: splitPath returned empty segment for path %q", path))
		}
		isLast := i == len(parts)-1

		if strings.HasPrefix(part, "*") {
			key := part[1:]
			if current.catchAll == nil {
				current.catchAll = &node{path: part, nType: catchAllNode, paramKey: key, fullPath: path}
			}
			if isLast {
				overwritten = current.catchAll.handlers != nil
				current.catchAll.handlers = handlers
				current.catchAll.fullPath = path
			}
			break
		}

		// {name:pattern} — regex-constrained parameter
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && strings.Contains(part, ":") {
			inner := part[1 : len(part)-1] // strip { }
			colonIdx := strings.Index(inner, ":")
			key := inner[:colonIdx]
			pattern := inner[colonIdx+1:]
			if key == "" || pattern == "" {
				panic(fmt.Sprintf("astra: invalid regex segment %q in path %q", part, path))
			}
			re, err := getOrCompileRegexp(pattern)
			if err != nil {
				panic(fmt.Sprintf("astra: invalid regex %q in path %q: %v", pattern, path, err))
			}
			child := findRegexChild(current, re)
			if child == nil {
				child = &node{
					path:      part,
					nType:     regexNode,
					paramKey:  key,
					regex:     re,
					fastMatch: compileFastMatcher(pattern),
				}
				current.regexChildren = append(current.regexChildren, child)
			}
			if isLast {
				overwritten = child.handlers != nil
				child.handlers = handlers
				child.fullPath = path
			}
			current = child
			continue
		}

		if strings.HasPrefix(part, ":") {
			key := part[1:]
			if current.param == nil {
				current.param = &node{path: part, nType: paramNode, paramKey: key, fullPath: path}
			}
			if isLast {
				overwritten = current.param.handlers != nil
				current.param.handlers = handlers
				current.param.fullPath = path
			}
			current = current.param
		} else {
			child := findStaticChild(current, part)
			if child == nil {
				child = &node{path: part, nType: staticNode, fullPath: path}
				current.children = append(current.children, child)
				recordChildIndex(current, len(current.children)-1)
			}
			if isLast {
				overwritten = child.handlers != nil
				child.handlers = handlers
				child.fullPath = path
			}
			current = child
		}
	}
	return
}

// findRegexChild finds an existing regex child that shares the same compiled
// Regexp instance (pointer equality after cache lookup).
func findRegexChild(n *node, re *regexp.Regexp) *node {
	for _, child := range n.regexChildren {
		if child.regex == re {
			return child
		}
	}
	return nil
}

func findStaticChild(n *node, segment string) *node {
	for _, child := range n.children {
		if child.path == segment {
			return child
		}
	}
	return nil
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// matchRoute traverses the trie to find handlers and extract params.
//
// params is passed in as the initial (pre-allocated) slice; it is typically
// c.params from Handle(), backed by c.paramsArr.  Passing a non-nil slice with
// sufficient capacity (≥ number of path parameters) means all append() calls
// inside matchSegments stay within the existing backing array — zero heap alloc.
//
// Path parsing is done inline (no splitPath / strings.Split call), eliminating
// two allocations on every request compared to the previous design.
func matchRoute(root *node, path string, params Params, maxParamValueLen int) (HandlersChain, Params, string, bool) {
	pos := 0
	if len(path) > 0 && path[0] == '/' {
		pos = 1
	}
	// Root path "/" or empty: check root node directly.
	if pos >= len(path) {
		if root.handlers != nil {
			return root.handlers, params, root.fullPath, true
		}
		return nil, nil, "", false
	}
	return matchSegments(root, path, pos, params, maxParamValueLen)
}

// matchSegments is the recursive heart of the router.
//
// It operates directly on the original path string using byte-offset slicing
// (path[a:b]) rather than a pre-split []string, so each call allocates nothing
// for segment extraction.  param values stored in Params are sub-slices of the
// input path string — valid for the lifetime of the http.Request.
func matchSegments(current *node, path string, pos int, params Params, maxParamValueLen int) (HandlersChain, Params, string, bool) {
	// Extract the current segment without allocating.
	// strings.IndexByte is O(k) on path[pos:] — no slice header escapes.
	end := strings.IndexByte(path[pos:], '/')
	var part string
	var nextPos int
	if end < 0 {
		part = path[pos:]       // sub-slice of original string: no alloc
		nextPos = len(path)
	} else {
		part = path[pos : pos+end] // sub-slice: no alloc
		nextPos = pos + end + 1    // skip the '/'
	}
	isLast := nextPos >= len(path)

	// Defensive guard: an empty segment (e.g. double "//" in the path) would
	// cause a panic below when we access part[0] for the childIndex dispatch.
	// Return false so the request falls through to 404 instead of crashing.
	if part == "" {
		return nil, nil, "", false
	}

	// Catch-all consumes the current segment and everything after it.
	// path[pos-1:] is a sub-slice of the original string — no alloc.
	// pos is always ≥1 here (guaranteed by matchRoute stripping the leading '/').
	if current.catchAll != nil && current.catchAll.handlers != nil {
		remaining := path[pos-1:] // "/segment/rest..." — no alloc
		if maxParamValueLen > 0 && len(remaining) > maxParamValueLen {
			return nil, nil, "", false
		}
		p := append(params, Param{Key: current.catchAll.paramKey, Value: remaining})
		return current.catchAll.handlers, p, current.catchAll.fullPath, true
	}

	// Try static children first (most specific match).
	// Fast path: use childIndex to skip to the right bucket in O(1).
	var matchedStatic *node
	if current.childIndex != nil && len(part) > 0 {
		switch idx := current.childIndex[part[0]]; {
		case idx >= 0:
			if current.children[idx].path == part {
				matchedStatic = current.children[idx]
			}
			// idx >= 0 but path mismatch: no other child can have this first byte.
		case idx == childIndexCollision:
			// Two or more children share this first byte; use childMap for O(1)
			// lookup when available, otherwise fall back to linear scan.
			if current.childMap != nil {
				matchedStatic = current.childMap[part]
			} else {
				for _, child := range current.children {
					if child.path == part {
						matchedStatic = child
						break
					}
				}
			}
		}
		// childIndexAbsent (-1): no child has this first byte; matchedStatic stays nil.
	} else {
		for _, child := range current.children {
			if child.path == part {
				matchedStatic = child
				break
			}
		}
	}
	if matchedStatic != nil {
		if isLast {
			if matchedStatic.handlers != nil {
				return matchedStatic.handlers, params, matchedStatic.fullPath, true
			}
			if matchedStatic.catchAll != nil && matchedStatic.catchAll.handlers != nil {
				p := append(params, Param{Key: matchedStatic.catchAll.paramKey, Value: "/"})
				return matchedStatic.catchAll.handlers, p, matchedStatic.catchAll.fullPath, true
			}
			return nil, nil, "", false
		}
		if h, p, fp, ok := matchSegments(matchedStatic, path, nextPos, params, maxParamValueLen); ok {
			return h, p, fp, ok
		}
	}

	// Try regex children (more specific than bare :param).
	// Fast-path matchers bypass the regexp engine for well-known patterns.
	for _, child := range current.regexChildren {
		var matched bool
		if child.fastMatch != nil {
			matched = child.fastMatch(part)
		} else {
			matched = child.regex != nil && child.regex.MatchString(part)
		}
		if matched {
			if maxParamValueLen > 0 && len(part) > maxParamValueLen {
				continue
			}
			newParams := append(params, Param{Key: child.paramKey, Value: part})
			if isLast {
				if child.handlers != nil {
					return child.handlers, newParams, child.fullPath, true
				}
				if child.catchAll != nil && child.catchAll.handlers != nil {
					p := append(newParams, Param{Key: child.catchAll.paramKey, Value: "/"})
					return child.catchAll.handlers, p, child.catchAll.fullPath, true
				}
				return nil, nil, "", false
			}
			if h, p, fp, ok := matchSegments(child, path, nextPos, newParams, maxParamValueLen); ok {
				return h, p, fp, ok
			}
		}
	}

	// Try param child.
	if current.param != nil {
		if maxParamValueLen == 0 || len(part) <= maxParamValueLen {
			newParams := append(params, Param{Key: current.param.paramKey, Value: part})
			if isLast {
				if current.param.handlers != nil {
					return current.param.handlers, newParams, current.param.fullPath, true
				}
				if current.param.catchAll != nil && current.param.catchAll.handlers != nil {
					p := append(newParams, Param{Key: current.param.catchAll.paramKey, Value: "/"})
					return current.param.catchAll.handlers, p, current.param.catchAll.fullPath, true
				}
				return nil, nil, "", false
			}
			if h, p, fp, ok := matchSegments(current.param, path, nextPos, newParams, maxParamValueLen); ok {
				return h, p, fp, ok
			}
		}
	}

	return nil, nil, "", false
}

// nodeParamDepth recursively walks the radix tree and returns the maximum
// accumulated param count along any root-to-leaf path.
func nodeParamDepth(n *node, depth int) int {
	if n.nType == paramNode || n.nType == regexNode {
		depth++
	}
	max := depth
	for _, child := range n.children {
		if d := nodeParamDepth(child, depth); d > max {
			max = d
		}
	}
	for _, child := range n.regexChildren {
		if d := nodeParamDepth(child, depth); d > max {
			max = d
		}
	}
	if n.param != nil {
		if d := nodeParamDepth(n.param, depth); d > max {
			max = d
		}
	}
	if n.catchAll != nil {
		if d := nodeParamDepth(n.catchAll, depth+1); d > max {
			max = d
		}
	}
	return max
}

func collectRoutes(n *node, prefix string, method string, routes *[]RouteInfo) {
	var path string
	if n.path == "/" {
		path = "/"
	} else {
		path = prefix + "/" + n.path
	}
	if n.handlers != nil && n.path != "/" {
		*routes = append(*routes, RouteInfo{Method: method, Path: path, FullPath: n.fullPath})
	} else if n.handlers != nil && n.path == "/" {
		*routes = append(*routes, RouteInfo{Method: method, Path: "/", FullPath: n.fullPath})
	}
	for _, child := range n.children {
		collectRoutes(child, path, method, routes)
	}
	for _, child := range n.regexChildren {
		collectRoutes(child, path, method, routes)
	}
	if n.param != nil {
		collectRoutes(n.param, path, method, routes)
	}
	if n.catchAll != nil {
		collectRoutes(n.catchAll, path, method, routes)
	}
}
