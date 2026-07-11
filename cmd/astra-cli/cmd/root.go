package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var GlobalOutDir string
var GlobalForce bool

// NewRootCmd creates the root cobra command.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "astra-cli",
		Short: "astra-cli — interactive CLI scaffolding tool for the Astra framework",
		Long: `astra-cli is a user-friendly scaffolding tool for the Astra Go framework.

It helps you quickly bootstrap new services and generate boilerplate code
through interactive prompts, aligning with Astra's idiomatic patterns.

Commands:
  astra-cli init          Initialize an existing project (go mod + directory structure)
  astra-cli new           Scaffold a new service with DI setup + Makefile
  astra-cli generate      Code generation commands

Run 'astra-cli <command> --help' for details on each command.`,
		Version: version,
	}

	// Global flags shared by all subcommands.
	root.PersistentFlags().StringVarP(&GlobalOutDir, "out", "o", "", "output directory (default: stdout or current directory)")
	root.PersistentFlags().BoolVarP(&GlobalForce, "force", "f", false, "overwrite existing files without prompting")

	// Register subcommands.
	root.AddCommand(newNewCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newGenerateCmd())

	return root
}

// fatal prints msg to stderr and exits with code 1.
func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "astra-cli:", msg)
	os.Exit(1)
}

// dirExists reports whether path already exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// FileExists reports whether path exists (any type).
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MkdirAll creates dir, printing an error and exiting on failure.
func MkdirAll(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		fatal("mkdir " + dir + ": " + err.Error())
	}
}
