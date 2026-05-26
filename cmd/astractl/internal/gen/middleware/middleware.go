package middleware

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
			Example: "astractl gen middleware Auth --dir ./internal/middleware",
		}
	}
	name := args[0]

	if !tpldata.IsValidGoIdent(tpldata.Pascal(name)) {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid name: %q — Pascal(%q) = %q is not a valid Go identifier", name, name, tpldata.Pascal(name)),
			Hint:    "use letters, digits, hyphens, or underscores (e.g. rate-limit, RateLimit)",
			Example: "astractl gen middleware RateLimit --dir ./internal/middleware",
		}
	}

	fs := flag.NewFlagSet("gen middleware", flag.ContinueOnError)
	dir   := fs.String("dir",   "", "output directory")
	pkg   := fs.String("pkg",   "middleware", "Go package name")
	force := fs.Bool("force", false, "overwrite existing file")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen middleware --help' to see all available flags",
		}
	}

	filename := fmt.Sprintf("%s_middleware.go", strings.ToLower(name))
	if err := fsutil.WriteTemplate(*dir, filename, tmpl.Middleware(), tpldata.New(name, "", *pkg), *force); err != nil {
		return err
	}

	if *dir != "" {
		fmt.Printf("Middleware generated: %s/%s\n", *dir, filename)
	} else {
		fmt.Printf("Middleware generated: %s\n", filename)
	}
	return nil
}
