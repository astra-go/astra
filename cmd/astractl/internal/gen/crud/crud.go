package crud

import (
	"flag"
	"fmt"
	"path/filepath"
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
			Example: "astractl gen crud Product --dir ./internal --with-service",
		}
	}
	name := args[0]

	if !tpldata.IsValidGoIdent(tpldata.Pascal(name)) {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("invalid name: %q — Pascal(%q) = %q is not a valid Go identifier", name, name, tpldata.Pascal(name)),
			Hint:    "use letters, digits, hyphens, or underscores (e.g. product-order, ProductOrder)",
			Example: "astractl gen crud ProductOrder --dir ./internal",
		}
	}

	fs := flag.NewFlagSet("gen crud", flag.ContinueOnError)
	base        := fs.String("dir",          "", "base output directory; subdirs handler/model/repository are created inside")
	withService := fs.Bool("with-service",   false, "also generate a service layer file")
	force       := fs.Bool("force",          false, "overwrite existing files")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen crud --help' to see all available flags",
		}
	}

	nameL := strings.ToLower(name)

	handlerDir := filepath.Join(*base, "handler")
	modelDir   := filepath.Join(*base, "model")
	repoDir    := filepath.Join(*base, "repository")

	handlerFile := fmt.Sprintf("%s_handler.go", nameL)
	modelFile   := fmt.Sprintf("%s_model.go", nameL)
	repoFile    := fmt.Sprintf("%s_repo.go", nameL)

	tpl := tmpl.Handler()
	if *withService {
		tpl = tmpl.HandlerWithService()
	}

	if err := fsutil.WriteTemplate(handlerDir, handlerFile, tpl, tpldata.New(name, "", "handler"), *force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(modelDir, modelFile, tmpl.Model(), tpldata.New(name, "", "model"), *force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(repoDir, repoFile, tmpl.Repo(), tpldata.New(name, "", "repository"), *force); err != nil {
		return err
	}

	fmt.Printf("CRUD scaffold generated:\n")
	fmt.Printf("  %s\n", filepath.Join(handlerDir, handlerFile))
	fmt.Printf("  %s\n", filepath.Join(modelDir, modelFile))
	fmt.Printf("  %s\n", filepath.Join(repoDir, repoFile))

	if *withService {
		serviceDir  := filepath.Join(*base, "service")
		serviceFile := fmt.Sprintf("%s_service.go", nameL)
		if err := fsutil.WriteTemplate(serviceDir, serviceFile, tmpl.Service(), tpldata.New(name, "", "service"), *force); err != nil {
			return err
		}
		fmt.Printf("  %s\n", filepath.Join(serviceDir, serviceFile))
	}
	return nil
}
