//go:build go1.25

package astra

// GoVersion125 indicates the runtime Go version is at least 1.25.
// The build tag uses go1.25 (not go1.25.1) because Go's toolchain only
// recognises major.minor in build constraints; patch versions are ignored.
const GoVersion125 = true
