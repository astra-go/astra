package cmd

import (
	"github.com/spf13/cobra"
)

// newGenerateCmd creates the `astra-cli generate` parent command.
func newGenerateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "generate",
		Short: "Generate code stubs (endpoint, crud, middleware)",
		Long: `Generate boilerplate code from OpenAPI specs, database schemas,
or templates aligned with Astra's conventions.

Subcommands:
  endpoint    Generate HTTP handlers from OpenAPI/Swagger specs
  crud        Generate CRUD handler + model + repository from table schema
  middleware  Generate a middleware scaffold

All subcommands support the following global flags:
  -o, --out   output directory (default: current directory)
  -f, --force overwrite existing files without prompting

Use 'astra-cli generate <subcommand> --help' for subcommand-specific options.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	c.AddCommand(newGenerateEndpointCmd())
	c.AddCommand(newGenerateCrudCmd())
	c.AddCommand(newGenerateMiddlewareCmd())

	return c
}
