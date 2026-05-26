package container

import (
	"flag"
	"fmt"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("gen container", flag.ContinueOnError)
	dir   := fs.String("dir",   ".", "output directory")
	force := fs.Bool("force", false, "overwrite existing file")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen container --help' to see all available flags",
		}
	}

	if err := fsutil.WriteTemplate(*dir, "container.go", tmpl.DIContainer(), tpldata.Data{}, *force); err != nil {
		return err
	}
	fmt.Printf("DI container generated: %s/container.go\n", *dir)
	fmt.Println("  Call initContainer(app) in main() before app.Run()")
	fmt.Println("  Requires: go get github.com/astra-go/astra/di")
	return nil
}
