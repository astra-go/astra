package model

import (
	"flag"
	"fmt"
	"strings"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

func Run(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return &cli.CLIError{
			Msg:     "missing required argument: <name>",
			Example: "astractl gen model User --dir ./internal/model",
		}
	}
	name := args[0]

	if !tpldata.IsValidGoIdent(tpldata.Pascal(name)) {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid name: %q — Pascal(%q) = %q is not a valid Go identifier", name, name, tpldata.Pascal(name)),
			Hint:    "use letters, digits, hyphens, or underscores (e.g. product-order, ProductOrder)",
			Example: "astractl gen model ProductOrder --dir ./internal/model",
		}
	}

	fs := flag.NewFlagSet("gen model", flag.ContinueOnError)
	dir   := fs.String("dir",   "", "output directory")
	pkg   := fs.String("pkg",   "model", "Go package name")
	force := fs.Bool("force", false, "overwrite existing file")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen model --help' to see all available flags",
		}
	}

	filename := fmt.Sprintf("%s_model.go", strings.ToLower(name))
	if err := fsutil.WriteTemplate(*dir, filename, tmpl.Model(), tpldata.New(name, "", *pkg), *force); err != nil {
		return err
	}

	if *dir != "" {
		fmt.Printf("Model generated: %s/%s\n", *dir, filename)
	} else {
		fmt.Printf("Model generated: %s\n", filename)
	}
	return nil
}
