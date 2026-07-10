// Package tpldata provides the shared template data struct used across all
// astra-cli code-generation templates.
package tpldata

import (
	"strings"
	"time"
	"unicode"
)

// Data is the context object passed to every template.
type Data struct {
	Name           string // Pascal-case entity/project name, e.g. "ArticleService"
	NameLower      string // lowercase name, e.g. "articleservice"
	Module         string // Go module path, e.g. "github.com/myorg/my-api"
	Layout         string // "simple" | "ddd"
	WithDocker     bool
	WithCI         bool
	Columns        []Column // for CRUD generation
	MiddlewareType string   // "auth" | "logging" | "rate-limit" | "cors" | "custom"
	Year           int
}

// Column describes a database column for CRUD generation.
type Column struct {
	Name    string // Go field name (PascalCase), e.g. "Title"
	JSONTag string // e.g. `"title"`
	GoType  string // e.g. "string", "int64"
	GORMCol string // gorm:"column:title;size:255"
	Comment string // e.g. "// article title"
}

// New builds a Data from a name and module path.
func New(name, module string) Data {
	return Data{
		Name:      Pascal(name),
		NameLower: strings.ToLower(name),
		Module:    module,
		Year:      time.Now().Year(),
	}
}

// Pascal converts "my-name" or "my_name" to "MyName".
func Pascal(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			runes := []rune(w)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, "")
}
