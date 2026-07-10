package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/astra-go/astra/cmd/astra-cli/internal/fsutil"
	"github.com/astra-go/astra/cmd/astra-cli/internal/tpldata"
	"github.com/astra-go/astra/cmd/astra-cli/internal/templates"
)

// newCmd represents the `astra-cli new` command.
func newNewCmd() *cobra.Command {
	var (
		interactive bool
		optName     string
		optModule   string
		optLayout   string
		optWithDocker bool
		optWithCI  bool
	)

	c := &cobra.Command{
		Use:   "new [service-name]",
		Short: "Scaffold a new Astra service (main.go + DI setup + Makefile)",
		Long: `Scaffolds a new Astra microservice with a main.go, DI container,
Makefile, config files, and optional Docker/CI support.

If no arguments are provided it runs in interactive mode and asks for all
required inputs. You can also pass them via flags to skip the prompts:

  astra-cli new my-api --module github.com/myorg/my-api --layout simple

Supported layouts: simple (single binary) | ddd (domain-driven design)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// ── Resolve inputs ──────────────────────────────────────────────
			var name, module, layout string
			var withDocker, withCI bool
			var err error

			if interactive || len(args) == 0 {
				name, module, layout, withDocker, withCI, err = promptsNew()
				if err != nil {
					return fmt.Errorf("interactive prompt cancelled: %w", err)
				}
			} else {
				name = args[0]
				if optName != "" {
					name = optName
				}
				if name == "" {
					return fmt.Errorf("service name is required")
				}
				if optModule != "" {
					module = optModule
				} else {
					module = name
				}
				layout = optLayout
				if layout == "" {
					layout = "simple"
				}
				withDocker = optWithDocker
				withCI = optWithCI
			}

			// ── Validate ───────────────────────────────────────────────────
			if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
				return fmt.Errorf("invalid service name %q: must not contain path separators", name)
			}
			if layout != "simple" && layout != "ddd" {
				return fmt.Errorf("unsupported layout %q: use 'simple' or 'ddd'", layout)
			}

			// ── Prepare template data ───────────────────────────────────────
			data := tpldata.New(name, module)
			data.Layout = layout
			data.WithDocker = withDocker
			data.WithCI = withCI

			// ── Determine output root ──────────────────────────────────────
			// Respect --out flag if set, otherwise create ./<name>
			outRoot := name
			if globalOutDir != "" {
				outRoot = globalOutDir
			}

			// Guard: do not accidentally overwrite a non-empty directory.
			if dirExists(outRoot) && !globalForce {
				entries, _ := os.ReadDir(outRoot)
				if len(entries) > 0 {
					return fmt.Errorf("directory %q already exists and is not empty (use --force to overwrite)", outRoot)
				}
			}

			// ── Write project files ───────────────────────────────────────

			// Shared files for all layouts
			mkdirAll(filepath.Join(outRoot, "config"))
			mkdirAll(filepath.Join(outRoot, "internal", "handler"))
			mkdirAll(filepath.Join(outRoot, "internal", "service"))
			mkdirAll(filepath.Join(outRoot, "internal", "model"))
			mkdirAll(filepath.Join(outRoot, "internal", "middleware"))
			mkdirAll(filepath.Join(outRoot, "internal", "repository"))
			mkdirAll(filepath.Join(outRoot, "internal", "dto"))

			if err := fsutil.WriteTemplate(outRoot, "go.mod", templates.GoMod(), data); err != nil {
				return fmt.Errorf("write go.mod: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, ".gitignore", templates.Gitignore(), data); err != nil {
				return fmt.Errorf("write .gitignore: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, "Makefile", templates.Makefile(), data); err != nil {
				return fmt.Errorf("write Makefile: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, "config", filepath.Join("config", "dev.yaml"), templates.ConfigDev(), data); err != nil {
				return fmt.Errorf("write config/dev.yaml: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, "config", filepath.Join("config", "prod.yaml"), templates.ConfigProd(), data); err != nil {
				return fmt.Errorf("write config/prod.yaml: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "handler"), "handler.go", templates.Handler(), data); err != nil {
				return fmt.Errorf("write handler: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "service"), "service.go", templates.Service(), data); err != nil {
				return fmt.Errorf("write service: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "model"), "model.go", templates.Model(), data); err != nil {
				return fmt.Errorf("write model: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "dto"), "dto.go", templates.DTO(), data); err != nil {
				return fmt.Errorf("write dto: %w", err)
			}

			if layout == "simple" {
				if err := fsutil.WriteTemplate(outRoot, "main.go", templates.MainSimple(), data); err != nil {
					return fmt.Errorf("write main.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, "wire.go", templates.WireProvider(), data); err != nil {
					return fmt.Errorf("write wire.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, "container.go", templates.Container(), data); err != nil {
					return fmt.Errorf("write container.go: %w", err)
				}
			} else {
				// ddd layout
				mkdirAll(filepath.Join(outRoot, "cmd", "server"))
				mkdirAll(filepath.Join(outRoot, "internal", "domain"))
				mkdirAll(filepath.Join(outRoot, "internal", "application"))
				mkdirAll(filepath.Join(outRoot, "internal", "infrastructure"))
				mkdirAll(filepath.Join(outRoot, "pkg", "errors"))

				if err := fsutil.WriteTemplate(outRoot, filepath.Join("cmd", "server"), "main.go", templates.MainDDD(), data); err != nil {
					return fmt.Errorf("write cmd/server/main.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, filepath.Join("cmd", "server"), "wire.go", templates.WireProvider(), data); err != nil {
					return fmt.Errorf("write wire.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, filepath.Join("cmd", "server"), "container.go", templates.Container(), data); err != nil {
					return fmt.Errorf("write container.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, filepath.Join("pkg", "errors"), "errors.go", templates.ErrorCodes(), data); err != nil {
					return fmt.Errorf("write errors: %w", err)
				}
			}

			if withDocker {
				mkdirAll(filepath.Join(outRoot, "deploy", "docker"))
				if err := fsutil.WriteTemplate(outRoot, "Dockerfile", templates.Dockerfile(), data); err != nil {
					return fmt.Errorf("write Dockerfile: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, "docker-compose.yml", templates.DockerCompose(), data); err != nil {
					return fmt.Errorf("write docker-compose.yml: %w", err)
				}
			}

			if withCI {
				mkdirAll(filepath.Join(outRoot, ".github", "workflows"))
				if err := fsutil.WriteTemplate(outRoot, filepath.Join(".github", "workflows"), "ci.yml", templates.CIWorkflow(), data); err != nil {
					return fmt.Errorf("write .github/workflows/ci.yml: %w", err)
				}
			}

			// ── Summary ──────────────────────────────────────────────────────
			fmt.Printf(`
✓ Service scaffolded: %s/  (layout: %s)
  Module: %s

Next steps:
  cd %s
  go mod tidy
  go generate ./...
  go run ./cmd/server/...
`, outRoot, layout, module, outRoot)
			if withDocker {
				fmt.Println("  docker compose up  # start Postgres + Redis")
			}
			return nil
		},
	}

	c.Flags().BoolVar(&interactive, "interactive", false, "run in interactive mode (default when no args provided)")
	c.Flags().StringVar(&optName, "name", "", "service name")
	c.Flags().StringVar(&optModule, "module", "", "Go module path (default: same as name)")
	c.Flags().StringVar(&optLayout, "layout", "simple", "project layout: simple | ddd")
	c.Flags().BoolVar(&optWithDocker, "docker", false, "include Dockerfile and docker-compose.yml")
	c.Flags().BoolVar(&optWithCI, "ci", false, "include GitHub Actions CI workflow")

	return c
}

// promptsNew runs interactive prompts and returns the gathered values.
func promptsNew() (name, module, layout string, withDocker, withCI bool, err error) {
	fmt.Println("=== astra-cli new — interactive mode ===")
	fmt.Println()

	name, err = promptString("Service name", "my-service")
	if err != nil {
		return
	}
	module, err = promptString("Go module path", name)
	if err != nil {
		return
	}
	layout, err = promptSelect("Project layout", []string{"simple", "ddd"}, "simple")
	if err != nil {
		return
	}
	withDocker, err = promptConfirm("Include Dockerfile and docker-compose.yml", false)
	if err != nil {
		return
	}
	withCI, err = promptConfirm("Include GitHub Actions CI workflow", false)
	if err != nil {
		return
	}
	return
}
