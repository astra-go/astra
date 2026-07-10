package generate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/astra-go/astra/cmd/astra-cli/internal/fsutil"
	"github.com/astra-go/astra/cmd/astra-cli/internal/tpldata"
	"github.com/astra-go/astra/cmd/astra-cli/internal/templates"
)

// newGenerateMiddlewareCmd creates the `astra-cli generate middleware` command.
func newGenerateMiddlewareCmd() *cobra.Command {
	var (
		interactive bool
		optName     string
		optDir      string
		optType     string
	)

	c := &cobra.Command{
		Use:   "middleware [name]",
		Short: "Generate a middleware scaffold",
		Long: `Generates an Astra middleware scaffold file. You can specify the middleware
type (auth, logging, rate-limit, cors, custom) or use "custom" for a blank template.

The generated file includes:
  - A named middleware function
  - Before-handler and after-handler sections marked with TODO comments
  - Proper context propagation helpers
  - Integration notes for DI

Examples:
  astra-cli generate middleware Auth
  astra-cli generate middleware RateLimit --type rate-limit
  astra-cli generate middleware --interactive`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			var mwType string
			var err error

			if interactive || len(args) == 0 {
				name, mwType, err = promptsMiddleware()
				if err != nil {
					return fmt.Errorf("interactive prompt cancelled: %w", err)
				}
			} else {
				name = args[0]
				if optName != "" {
					name = optName
				}
				mwType = optType
			}

			if name == "" {
				return errors.New("middleware name is required")
			}
			if !isValidGoIdent(pascal(name)) {
				return fmt.Errorf("invalid name: %q", name)
			}
			if mwType == "" {
				mwType = "custom"
			}

			validTypes := map[string]bool{
				"custom":      true,
				"auth":        true,
				"logging":     true,
				"rate-limit":  true,
				"cors":        true,
				"recovery":    true,
				"request-id":  true,
			}
			if !validTypes[mwType] {
				return fmt.Errorf("unsupported middleware type: %q (use: auth, logging, rate-limit, cors, recovery, request-id, custom)", mwType)
			}

			outDir := optDir
			if outDir == "" && globalOutDir != "" {
				outDir = globalOutDir
			}

			data := tpldata.New(name, "")
			data.MiddlewareType = mwType

			filename := strings.ToLower(pascal(name)) + "_middleware.go"
			if outDir != "" {
				mkdirAll(outDir)
				filename = filepath.Join(outDir, filename)
			}

			if fileExists(filename) && !globalForce {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", filename)
			}

			content, err := templates.RenderMiddleware(data)
			if err != nil {
				return fmt.Errorf("render middleware template: %w", err)
			}
			if err := fsutil.WriteString(filename, content); err != nil {
				return fmt.Errorf("write %s: %w", filename, err)
			}

			fmt.Printf("\n✓ Middleware generated: %s\n", filename)
			fmt.Printf("  Type: %s\n", mwType)
			fmt.Printf("  Function: %s()\n", pascal(name))
			fmt.Println("\nNext steps:")
			fmt.Printf("  1. Edit %s and implement your logic\n", filename)
			fmt.Printf("  2. Register it in your app: app.Use(%s())\n", pascal(name))
			if mwType == "custom" {
				fmt.Println("  3. Tip: add dependencies via parameters or closure")
			}
			return nil
		},
	}

	c.Flags().BoolVar(&interactive, "interactive", false, "run in interactive mode")
	c.Flags().StringVarP(&optName, "name", "n", "", "middleware name")
	c.Flags().StringVarP(&optDir, "dir", "d", "", "output directory")
	c.Flags().StringVar(&optType, "type", "custom", "middleware type: auth|logging|rate-limit|cors|recovery|request-id|custom")

	return c
}

func promptsMiddleware() (name, mwType string, err error) {
	fmt.Println("=== astra-cli generate middleware — interactive mode ===")
	fmt.Println()

	name, err = promptString("Middleware name", "MyMiddleware")
	if err != nil {
		return
	}

	typeOptions := []string{
		"custom",
		"auth",
		"logging",
		"rate-limit",
		"cors",
		"recovery",
		"request-id",
	}
	mwType, err = promptSelect("Middleware type", typeOptions, "custom")
	if err != nil {
		return
	}
	return
}
