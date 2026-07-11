package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/astra-go/astra/cmd/astra-cli/internal/fsutil"
	"github.com/astra-go/astra/cmd/astra-cli/internal/tpldata"
	"github.com/astra-go/astra/cmd/astra-cli/internal/templates"
)

// newInitCmd creates the `astra-cli init` command.
func newInitCmd() *cobra.Command {
	var (
		interactive   bool
		optModule     string
		optWithDocker bool
		optWithCI     bool
	)

	c := &cobra.Command{
		Use:   "init",
		Short: "Initialize an existing project (go mod init + standard directory structure)",
		Long: `Initializes a Go project in the current directory with Astra's
standard directory layout, a go.mod (via 'go mod init'), and optional
Docker/CI support.

If --module is not provided it is inferred from the current directory name.
Interactive mode asks for all options before proceeding.

Usage:
  astra-cli init
  astra-cli init --module github.com/myorg/my-api --docker --ci`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var module string
			var withDocker, withCI bool
			var err error

			if interactive {
				module, withDocker, withCI, err = promptsInit()
				if err != nil {
					return fmt.Errorf("interactive prompt cancelled: %w", err)
				}
			} else {
				module = optModule
				withDocker = optWithDocker
				withCI = optWithCI

				if module == "" {
					cwd, _ := os.Getwd()
					module = filepath.Base(cwd)
				}
			}

			outRoot := "."
			if GlobalOutDir != "" {
				outRoot = GlobalOutDir
			}

			goModPath := filepath.Join(outRoot, "go.mod")
			if FileExists(goModPath) && !GlobalForce {
				return fmt.Errorf("go.mod already exists in %s (use --force to overwrite)", outRoot)
			}

			// Run go mod init
			fmt.Printf("▶  go mod init %s\n", module)
			goBin, err := findGoBin()
			if err != nil {
				return fmt.Errorf("go binary not found: %w (is Go installed?)", err)
			}
			runDir := outRoot
			if outRoot == "." {
				runDir, _ = os.Getwd()
			}
			execCmd := exec.Command(goBin, "mod", "init", module)
			execCmd.Dir = runDir
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			if err := execCmd.Run(); err != nil {
				return fmt.Errorf("go mod init failed: %w", err)
			}

			name := filepath.Base(module)
			data := tpldata.New(name, module)
			data.WithDocker = withDocker
			data.WithCI = withCI

			dirs := []string{
				"cmd/server",
				"config",
				"internal/handler",
				"internal/service",
				"internal/model",
				"internal/dto",
				"internal/repository",
				"internal/middleware",
				"migrations",
			}
			for _, d := range dirs {
				MkdirAll(filepath.Join(outRoot, d))
			}

			// files: {relPath, renderedContent}
			type fileEntry struct{ path, content string }
			files := []fileEntry{
				{"Makefile", templates.RenderMakefile(data)},
				{".gitignore", templates.Gitignore()},
				{"config/dev.yaml", templates.RenderConfigDev(data)},
				{"config/prod.yaml", templates.RenderConfigProd(data)},
				{"cmd/server/main.go", templates.RenderMainDDD(data)},
				{"cmd/server/wire.go", templates.RenderWireProvider(data)},
				{"cmd/server/container.go", templates.RenderContainer(data)},
				{"internal/handler/handler.go", templates.RenderHandler(data)},
				{"internal/service/service.go", templates.RenderService(data)},
				{"internal/model/model.go", templates.RenderModel(data)},
				{"internal/dto/dto.go", templates.RenderDTO(data)},
				{"pkg/errors/errors.go", templates.RenderErrorCodes(data)},
			}

			var created []string
			for _, f := range files {
				fullPath := filepath.Join(outRoot, f.path)
				if FileExists(fullPath) && !GlobalForce {
					continue
				}
				fsutil.MkdirForFile(fullPath)
				if err := fsutil.WriteString(fullPath, f.content); err != nil {
					return fmt.Errorf("write %s: %w", f.path, err)
				}
				created = append(created, f.path)
			}

			if withDocker {
				MkdirAll(filepath.Join(outRoot, "deploy", "docker"))
				dockerFiles := []fileEntry{
					{"Dockerfile", templates.Dockerfile()},
					{"docker-compose.yml", templates.RenderDockerCompose(data)},
				}
				for _, f := range dockerFiles {
					fsutil.MkdirForFile(filepath.Join(outRoot, f.path))
					fsutil.WriteString(filepath.Join(outRoot, f.path), f.content)
					created = append(created, f.path)
				}
			}

			if withCI {
				MkdirAll(filepath.Join(outRoot, ".github", "workflows"))
				p := filepath.Join(outRoot, ".github/workflows/ci.yml")
				fsutil.WriteString(p, templates.RenderCIWorkflow(data))
				created = append(created, ".github/workflows/ci.yml")
			}

			fmt.Printf("\n✓ Project initialized: %s\n", outRoot)
			fmt.Printf("  Module: %s\n", module)
			fmt.Println("\nFiles created:")
			for _, f := range created {
				fmt.Printf("  %s\n", f)
			}
			fmt.Println("\nNext steps:")
			fmt.Println("  go mod tidy")
			fmt.Println("  go generate ./...")
			fmt.Println("  go run ./cmd/server/...")

			return nil
		},
	}

	c.Flags().BoolVar(&interactive, "interactive", false, "run in interactive mode")
	c.Flags().StringVar(&optModule, "module", "", "Go module path (default: inferred from directory name)")
	c.Flags().BoolVar(&optWithDocker, "docker", false, "include Dockerfile and docker-compose.yml")
	c.Flags().BoolVar(&optWithCI, "ci", false, "include GitHub Actions CI workflow")

	return c
}

func promptsInit() (module string, withDocker, withCI bool, err error) {
	fmt.Println("=== astra-cli init — interactive mode ===")
	fmt.Println()
	cwd, _ := os.Getwd()
	defaultModule := filepath.Base(cwd)
	module, err = PromptString("Go module path", defaultModule)
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

func findGoBin() (string, error) {
	paths := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range paths {
		candidate := filepath.Join(dir, "go")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("go not found in PATH")
}
