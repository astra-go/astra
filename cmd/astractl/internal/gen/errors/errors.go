package errors

import (
	"flag"
	"fmt"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("gen errors", flag.ContinueOnError)
	dir   := fs.String("dir",   ".", "output directory")
	pkg   := fs.String("pkg",   "errors", "Go package name")
	force := fs.Bool("force", false, "overwrite existing file")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen errors --help' to see all available flags",
		}
	}

	if err := fsutil.WriteTemplate(*dir, "errors.go", tmpl.ErrorCodes(), tpldata.New("", "", *pkg), *force); err != nil {
		return err
	}
	if *dir != "." {
		fmt.Printf("Error codes generated: %s/errors.go\n", *dir)
	} else {
		fmt.Println("Error codes generated: errors.go")
	}
	return nil
}
