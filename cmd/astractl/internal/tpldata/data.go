package tpldata

import (
	"strings"
	"time"
	"unicode"
)

// Data is the template data struct passed to all code-generation templates.
type Data struct {
	Name        string
	NameLower   string
	Module      string
	Year        int
	ID          string
	IDStr       string
	Description string
	Pkg         string
	Layout      string // "simple" | "ddd"
}

// New builds a Data from name, module and pkg.
func New(name, module, pkg string) Data {
	return Data{
		Name:      Pascal(name),
		NameLower: strings.ToLower(name),
		Module:    module,
		Year:      time.Now().Year(),
		Pkg:       pkg,
	}
}

// Pascal converts "my-name" or "my_name" to "MyName".
func Pascal(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, "")
}

// IsValidGoIdent reports whether s is a non-empty, valid Go identifier
// (starts with a Unicode letter or underscore, followed by letters/digits/underscores).
func IsValidGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
