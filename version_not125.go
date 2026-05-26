//go:build !go1.25

package astra

// GoVersion125 indicates the runtime Go version is at least 1.25.
// Use this for conditional features that require Go 1.25+.
const GoVersion125 = false
