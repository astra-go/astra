// Package template provides the built-in project template registry for astractl.
package template

// Entry describes one built-in project template.
type Entry struct {
	Name        string
	Source      string // "builtin" or a remote URL
	Tags        []string
	Description string
}

// Builtin is the list of templates shipped with astractl.
var Builtin = []Entry{
	{
		Name:        "simple",
		Source:      "builtin",
		Tags:        []string{"http-api", "starter"},
		Description: "Flat-layout REST API (handler/service/repo/model)",
	},
	{
		Name:        "ddd",
		Source:      "builtin",
		Tags:        []string{"http-api", "ddd"},
		Description: "Domain-driven design layout (cmd/internal/domain/application/infrastructure)",
	},
	{
		Name:        "microservice",
		Source:      "builtin",
		Tags:        []string{"http-api", "grpc", "ci"},
		Description: "Microservice skeleton with GitHub Actions CI workflow",
	},
}
