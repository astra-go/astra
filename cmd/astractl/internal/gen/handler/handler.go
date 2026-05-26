package handler

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
			Example: "astractl gen handler User --dir ./internal/handler --service",
		}
	}
	name := args[0]

	if !tpldata.IsValidGoIdent(tpldata.Pascal(name)) {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid name: %q — Pascal(%q) = %q is not a valid Go identifier", name, name, tpldata.Pascal(name)),
			Hint:    "use letters, digits, hyphens, or underscores (e.g. user-profile, UserProfile)",
			Example: "astractl gen handler UserProfile --dir ./internal/handler",
		}
	}

	fs := flag.NewFlagSet("gen handler", flag.ContinueOnError)
	dir         := fs.String("dir",     "", "output directory (default: current directory)")
	pkg         := fs.String("pkg",     "handler", "Go package name")
	withService := fs.Bool("service",   false, "co-generate a service interface in the handler file")
	force       := fs.Bool("force",     false, "overwrite existing file")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen handler --help' to see all available flags",
		}
	}

	filename := fmt.Sprintf("%s_handler.go", strings.ToLower(name))
	data := tpldata.New(name, "", *pkg)

	tpl := tmpl.Handler()
	if *withService {
		tpl = tmpl.HandlerWithService()
	}

	if err := fsutil.WriteTemplate(*dir, filename, tpl, data, *force); err != nil {
		return err
	}

	if *dir != "" {
		fmt.Printf("Handler generated: %s/%s\n", *dir, filename)
	} else {
		fmt.Printf("Handler generated: %s\n", filename)
	}
	if *withService {
		fmt.Printf("  (includes %sService interface — implement it in your service layer)\n", tpldata.Pascal(name))
	}
	return nil
}
