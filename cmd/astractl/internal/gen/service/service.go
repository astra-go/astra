package service

import (
	"flag"
	"fmt"
	"strings"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/gen/svcproto"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

func Run(args []string) error {
	// Delegate to full-scaffold mode when --proto is present.
	for _, a := range args {
		if a == "--proto" || strings.HasPrefix(a, "--proto=") {
			return svcproto.Run(args)
		}
	}

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return &cli.CLIError{
			Msg:     "missing required argument: <name>",
			Example: "astractl gen service Payment --dir ./internal/service",
			Hint:    "use --proto <file.proto> to generate a full microservice skeleton",
		}
	}
	name := args[0]

	if !tpldata.IsValidGoIdent(tpldata.Pascal(name)) {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid name: %q — Pascal(%q) = %q is not a valid Go identifier", name, name, tpldata.Pascal(name)),
			Hint:    "use letters, digits, hyphens, or underscores (e.g. payment-gateway, PaymentGateway)",
			Example: "astractl gen service PaymentGateway --dir ./internal/service",
		}
	}

	fs := flag.NewFlagSet("gen service", flag.ContinueOnError)
	dir   := fs.String("dir",   "", "output directory")
	pkg   := fs.String("pkg",   "service", "Go package name")
	force := fs.Bool("force", false, "overwrite existing file")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen service --help' to see all available flags",
		}
	}

	filename := fmt.Sprintf("%s_service.go", strings.ToLower(name))
	if err := fsutil.WriteTemplate(*dir, filename, tmpl.Service(), tpldata.New(name, "", *pkg), *force); err != nil {
		return err
	}

	if *dir != "" {
		fmt.Printf("Service generated: %s/%s\n", *dir, filename)
	} else {
		fmt.Printf("Service generated: %s\n", filename)
	}
	return nil
}
