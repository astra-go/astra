// Package env generates IDE and development environment configuration files.
package env

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

// Run executes the gen env subcommand.
func Run(args []string) error {
	fs := flag.NewFlagSet("gen env", flag.ContinueOnError)
	ide          := fs.String("ide", "none", "IDE type: vscode | cursor | goland | none")
	lint         := fs.Bool("lint", false, "generate .golangci.yml")
	devcontainer := fs.Bool("devcontainer", false, "generate .devcontainer/devcontainer.json")
	hooks        := fs.Bool("hooks", false, "generate .githooks/pre-commit + scripts/install-hooks.sh")
	dir          := fs.String("dir", ".", "output directory (default: current directory)")
	force        := fs.Bool("force", false, "overwrite existing files")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{Msg: "invalid flags: " + err.Error()}
	}

	switch *ide {
	case "vscode", "cursor", "goland", "none":
	default:
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid --ide value: %q", *ide),
			Hint:    "--ide must be vscode, cursor, goland, or none",
			Example: "astractl gen env --ide vscode --lint",
		}
	}

	data := tpldata.New("app", "", "")

	// Derive project name from the output directory for devcontainer naming.
	if abs, err := filepath.Abs(*dir); err == nil {
		data.Name = filepath.Base(abs)
		data.NameLower = data.Name
	}

	var written []string

	// .editorconfig — always generated regardless of IDE choice.
	if err := fsutil.WriteTemplate(*dir, ".editorconfig", tmpl.EditorConfig(), data, *force); err != nil {
		return err
	}
	written = append(written, ".editorconfig")

	// IDE-specific files.
	switch *ide {
	case "vscode", "cursor":
		vsdir := filepath.Join(*dir, ".vscode")
		if err := os.MkdirAll(vsdir, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", vsdir, err)}
		}
		if err := fsutil.WriteTemplate(*dir, filepath.Join(".vscode", "settings.json"), tmpl.VSCodeSettings(), data, *force); err != nil {
			return err
		}
		written = append(written, ".vscode/settings.json")
		if err := fsutil.WriteTemplate(*dir, filepath.Join(".vscode", "launch.json"), tmpl.VSCodeLaunch(), data, *force); err != nil {
			return err
		}
		written = append(written, ".vscode/launch.json")
		if err := fsutil.WriteTemplate(*dir, filepath.Join(".vscode", "extensions.json"), tmpl.VSCodeExtensions(), data, *force); err != nil {
			return err
		}
		written = append(written, ".vscode/extensions.json")

	case "goland":
		ideaDir := filepath.Join(*dir, ".idea")
		if err := os.MkdirAll(ideaDir, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", ideaDir, err)}
		}
		fmt.Println("  hint: JetBrains IDE configuration is managed by the IDE itself.")
		fmt.Println("  Created directory: .idea/  — open the project in GoLand to populate it.")
	}

	// .golangci.yml
	if *lint {
		if err := fsutil.WriteTemplate(*dir, ".golangci.yml", tmpl.GolangCI(), data, *force); err != nil {
			return err
		}
		written = append(written, ".golangci.yml")
	}

	// .devcontainer/devcontainer.json
	if *devcontainer {
		dcDir := filepath.Join(*dir, ".devcontainer")
		if err := os.MkdirAll(dcDir, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", dcDir, err)}
		}
		if err := fsutil.WriteTemplate(*dir, filepath.Join(".devcontainer", "devcontainer.json"), tmpl.DevContainer(), data, *force); err != nil {
			return err
		}
		written = append(written, ".devcontainer/devcontainer.json")
	}

	// .githooks/pre-commit + scripts/install-hooks.sh
	if *hooks {
		hooksDir := filepath.Join(*dir, ".githooks")
		if err := os.MkdirAll(hooksDir, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", hooksDir, err)}
		}
		hookFile := filepath.Join(*dir, ".githooks", "pre-commit")
		if err := fsutil.WriteTemplate(*dir, filepath.Join(".githooks", "pre-commit"), tmpl.GitHookPreCommit(), data, *force); err != nil {
			return err
		}
		if err := os.Chmod(hookFile, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("chmod %s: %v", hookFile, err)}
		}
		written = append(written, ".githooks/pre-commit")

		scriptsDir := filepath.Join(*dir, "scripts")
		if err := os.MkdirAll(scriptsDir, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", scriptsDir, err)}
		}
		installSrc := "#!/bin/sh\n# Install project git hooks.\ngit config core.hooksPath .githooks\necho '✓ git hooks installed (.githooks/)'\n"
		if err := fsutil.WriteString(*dir, filepath.Join("scripts", "install-hooks.sh"), installSrc, *force); err != nil {
			return err
		}
		installScript := filepath.Join(*dir, "scripts", "install-hooks.sh")
		if err := os.Chmod(installScript, 0700); err != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("chmod %s: %v", installScript, err)}
		}
		written = append(written, "scripts/install-hooks.sh")
	}

	fmt.Printf("Dev environment files generated (%d file(s)):\n", len(written))
	for _, w := range written {
		fmt.Printf("  %s\n", w)
	}
	return nil
}
