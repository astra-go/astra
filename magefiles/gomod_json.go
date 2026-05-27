//go:build mage

package main

import (
	"encoding/json"
)

type goModJSON struct {
	Require []struct {
		Path     string
		Version  string
		Indirect bool
	}
}

// parseRequires parses the output of `go mod edit -json` and returns
// a map of module path → version for all direct and indirect requires.
func parseRequires(data []byte) map[string]string {
	var m goModJSON
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	result := make(map[string]string, len(m.Require))
	for _, r := range m.Require {
		result[r.Path] = r.Version
	}
	return result
}
