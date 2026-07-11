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
		interactive  bool
		optName      string
		optModule    string
		optLayout    string
		optWithDocker bool
		optWithCI    bool
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
			if GlobalOutDir != "" {
				outRoot = GlobalOutDir
			}

			// Guard: do not accidentally overwrite a non-empty directory.
			if dirExists(outRoot) && !GlobalForce {
				entries, _ := os.ReadDir(outRoot)
				if len(entries) > 0 {
					return fmt.Errorf("directory %q already exists and is not empty (use --force to overwrite)", outRoot)
				}
			}

			// ── Write project files ───────────────────────────────────────

			// Shared files for all layouts
			MkdirAll(filepath.Join(outRoot, "config"))
			MkdirAll(filepath.Join(outRoot, "internal", "handler"))
			MkdirAll(filepath.Join(outRoot, "internal", "service"))
			MkdirAll(filepath.Join(outRoot, "internal", "model"))
			MkdirAll(filepath.Join(outRoot, "internal", "middleware"))
			MkdirAll(filepath.Join(outRoot, "internal", "repository"))
			MkdirAll(filepath.Join(outRoot, "internal", "dto"))

			if err := fsutil.WriteTemplate(outRoot, "go.mod", templates.GoMod(), data); err != nil {
				return fmt.Errorf("write go.mod: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, ".gitignore", templates.Gitignore(), data); err != nil {
				return fmt.Errorf("write .gitignore: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, "Makefile", templates.Makefile(), data); err != nil {
				return fmt.Errorf("write Makefile: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("config", "dev.yaml"), templates.RenderConfigDev(data), data); err != nil {
				return fmt.Errorf("write config/dev.yaml: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("config", "prod.yaml"), templates.RenderConfigProd(data), data); err != nil {
				return fmt.Errorf("write config/prod.yaml: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "handler", "handler.go"), templates.RenderHandler(data), data); err != nil {
				return fmt.Errorf("write handler: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "service", "service.go"), templates.RenderService(data), data); err != nil {
				return fmt.Errorf("write service: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "model", "model.go"), templates.RenderModel(data), data); err != nil {
				return fmt.Errorf("write model: %w", err)
			}
			if err := fsutil.WriteTemplate(outRoot, filepath.Join("internal", "dto", "dto.go"), templates.RenderDTO(data), data); err != nil {
				return fmt.Errorf("write dto: %w", err)
			}

			if layout == "simple" {
				if err := fsutil.WriteTemplate(outRoot, "main.go", templates.RenderMainSimple(data), data); err != nil {
					return fmt.Errorf("write main.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, "wire.go", templates.RenderWireProvider(data), data); err != nil {
					return fmt.Errorf("write wire.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, "container.go", templates.RenderContainer(data), data); err != nil {
					return fmt.Errorf("write container.go: %w", err)
				}
			} else {
				// ddd layout
				MkdirAll(filepath.Join(outRoot, "cmd", "server"))
				MkdirAll(filepath.Join(outRoot, "internal", "domain"))
				MkdirAll(filepath.Join(outRoot, "internal", "application"))
				MkdirAll(filepath.Join(outRoot, "internal", "infrastructure"))
				MkdirAll(filepath.Join(outRoot, "pkg", "errors"))

				if err := fsutil.WriteTemplate(outRoot, filepath.Join("cmd", "server", "main.go"), templates.RenderMainDDD(data), data); err != nil {
					return fmt.Errorf("write cmd/server/main.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, filepath.Join("cmd", "server", "wire.go"), templates.RenderWireProvider(data), data); err != nil {
					return fmt.Errorf("write wire.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, filepath.Join("cmd", "server", "container.go"), templates.RenderContainer(data), data); err != nil {
					return fmt.Errorf("write container.go: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, filepath.Join("pkg", "errors", "errors.go"), templates.RenderErrorCodes(data), data); err != nil {
					return fmt.Errorf("write errors: %w", err)
				}
			}

			if withDocker {
				MkdirAll(filepath.Join(outRoot, "deploy", "docker"))
				if err := fsutil.WriteTemplate(outRoot, "Dockerfile", templates.Dockerfile(), data); err != nil {
					return fmt.Errorf("write Dockerfile: %w", err)
				}
				if err := fsutil.WriteTemplate(outRoot, "docker-compose.yml", templates.RenderDockerCompose(data), data); err != nil {
					return fmt.Errorf("write docker-compose.yml: %w", err)
				}
			}

			if withCI {
				MkdirAll(filepath.Join(outRoot, ".github", "workflows"))
				if err := fsutil.WriteTemplate(outRoot, filepath.Join(".github", "workflows", "ci.yml"), templates.RenderCIWorkflow(data), data); err != nil {
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
	c.Flags().StringVarP(&optName, "name", "", "service name", "")
	c.Flags().StringVarP(&optModule, "module", "", "Go module path (default: same as name)", "")
	c.Flags().StringVarP(&optLayout, "layout", "", "project layout: simple | ddd", "")
	c.Flags().BoolVarP(&optWithDocker, "docker", "", false, "include Dockerfile and docker-compose.yml")
	c.Flags().BoolVarP(&optWithCI, "ci", "", false, "include GitHub Actions CI workflow")

	return c
}

// promptsNew runs interactive prompts and returns the gathered values.
func promptsNew() (name, module, layout string, withDocker, withCI bool, err error) {
	fmt.Println("=== astra-cli new — interactive mode ===")
	fmt.Println()

	name, err = PromptString("Service name", "my-service")
	if err != nil {
		return
	}
	module, err = PromptString("Go module path", name)
	if err != nil {
		return
	}
	layout, err = PromptSelect("Project layout", []string{"simple", "ddd"}, "simple")
	if err != nil {
		return
	}
	withDocker, err = PromptConfirm("Include Dockerfile and docker-compose.yml", false)
	if err != nil {
		return
	}
	withCI, err = PromptConfirm("Include GitHub Actions CI workflow", false)
	if err != nil {
		return
	}
	return
}
