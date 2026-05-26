// Package svcproto generates a complete, runnable microservice skeleton from a .proto file.
// Invoked via: astractl gen service --proto <file.proto>
package svcproto

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	genproto "github.com/astra-go/astra/cmd/astractl/internal/gen/proto"
	"github.com/astra-go/astra/cmd/astractl/internal/tmpl"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

// Run implements: astractl gen service --proto <file.proto> [flags]
func Run(args []string) error {
	fs := flag.NewFlagSet("gen service --proto", flag.ContinueOnError)
	protoFile := fs.String("proto", "", "path to .proto file (required)")
	outDir    := fs.String("out-dir", "", "output directory (default: <FirstServiceName>-svc)")
	module    := fs.String("module", "", "Go module path (default: <out-dir>)")
	force     := fs.Bool("force", false, "overwrite existing files")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{
			Msg:  "invalid flags: " + err.Error(),
			Hint: "run 'astractl gen service --help' for usage",
		}
	}
	if *protoFile == "" {
		return &cli.CLIError{
			Msg:     "missing required flag: --proto",
			Example: "astractl gen service --proto api/service.proto --module github.com/myorg/my-svc",
		}
	}
	return generate(*protoFile, *outDir, *module, *force)
}

func generate(protoFile, outDir, module string, force bool) error {
	raw, err := os.ReadFile(protoFile)
	if err != nil {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("read %s: %v", protoFile, err),
			Hint:    "ensure the .proto file path is correct and readable",
			Example: "astractl gen service --proto api/service.proto",
		}
	}

	src := genproto.StripComments(string(raw))
	result := genproto.Parse(src, false)

	if len(result.Services) == 0 && len(result.Messages) == 0 {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("no service or message definitions found in %s", protoFile),
			Hint: "ensure the file contains valid proto3 service and message definitions",
			Example: `syntax = "proto3";
message GetUserRequest { string id = 1; }
service UserService { rpc GetUser(GetUserRequest) returns (GetUserResponse); }`,
		}
	}

	base := strings.TrimSuffix(filepath.Base(protoFile), ".proto")

	if outDir == "" {
		if len(result.Services) > 0 {
			outDir = strings.ToLower(result.Services[0].Name) + "-svc"
		} else {
			outDir = base + "-svc"
		}
	}
	if module == "" {
		module = outDir
	}

	// Create directory tree.
	for _, d := range []string{
		filepath.Join(outDir, "cmd", "server"),
		filepath.Join(outDir, "internal", "handler"),
		filepath.Join(outDir, "config"),
		filepath.Join(outDir, "migrations"),
		filepath.Join(outDir, ".github", "workflows"),
	} {
		if mkErr := os.MkdirAll(d, 0755); mkErr != nil {
			return &cli.CLIError{Msg: fmt.Sprintf("mkdir %s: %v", d, mkErr)}
		}
	}

	protoBase := filepath.Base(protoFile)
	handlerDir := filepath.Join(outDir, "internal", "handler")

	// 1. handler file: types + service interface + HTTP adapter.
	// Pass empty modulePath so BuildOutput defaults to "github.com/astra-go/astra".
	handlerSrc := genproto.BuildOutput(protoBase, "handler", "", "", false, false, result)
	if err := fsutil.WriteString(handlerDir, base+"_handler.go", handlerSrc, force); err != nil {
		return err
	}

	// 2. impl skeleton + test file (only when services are present).
	if len(result.Services) > 0 {
		implSrc := genproto.BuildImplSkeleton("handler", result.Services)
		if err := fsutil.WriteString(handlerDir, base+"_impl.go", implSrc, force); err != nil {
			return err
		}

		testSrc := buildTestFile(module, base, result)
		if err := fsutil.WriteString(handlerDir, base+"_handler_test.go", testSrc, force); err != nil {
			return err
		}
	}

	// 3. cmd/server/main.go.
	mainSrc := buildMainSrc(module, result.Services)
	if err := fsutil.WriteString(filepath.Join(outDir, "cmd", "server"), "main.go", mainSrc, force); err != nil {
		return err
	}

	// 4. Project scaffolding files (reuse existing templates).
	modData := tpldata.New(outDir, module, "")

	if err := fsutil.WriteTemplate(outDir, "go.mod", tmpl.GoMod(), modData, force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(outDir, "Makefile", tmpl.Makefile(), modData, force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(outDir, "Dockerfile", tmpl.Dockerfile(), modData, force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(outDir, ".gitignore", tmpl.Gitignore(), modData, force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(filepath.Join(outDir, ".github", "workflows"), "ci.yml", tmpl.CIWorkflow(), modData, force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(filepath.Join(outDir, "config"), "dev.yaml", tmpl.ConfigDev(), modData, force); err != nil {
		return err
	}
	if err := fsutil.WriteTemplate(filepath.Join(outDir, "config"), "prod.yaml", tmpl.ConfigProd(), modData, force); err != nil {
		return err
	}

	// Print summary.
	fmt.Printf("\nService skeleton generated: %s/\n", outDir)
	fmt.Printf("  Module:    %s\n", module)
	fmt.Printf("  Proto:     %s\n", protoFile)
	for _, svc := range result.Services {
		fmt.Printf("  Service:   %s (%d method(s))\n", svc.Name, len(svc.RPCs))
	}
	fmt.Printf("  Messages:  %d\n", len(result.Messages))
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", outDir)
	fmt.Println("  go mod tidy")
	fmt.Println("  go run ./cmd/server/...")
	return nil
}

// buildMainSrc generates cmd/server/main.go that wires all services from the handler package.
func buildMainSrc(module string, services []genproto.SvcInfo) string {
	var out strings.Builder
	hasServices := len(services) > 0

	fmt.Fprintf(&out, "package main\n\n")
	fmt.Fprintf(&out, "import (\n")
	fmt.Fprintf(&out, "\t\"net/http\"\n\n")
	fmt.Fprintf(&out, "\t\"github.com/astra-go/astra\"\n")
	fmt.Fprintf(&out, "\t\"github.com/astra-go/astra/middleware\"\n")
	if hasServices {
		fmt.Fprintf(&out, "\t%q\n", module+"/internal/handler")
	}
	fmt.Fprintf(&out, ")\n\n")

	fmt.Fprintf(&out, "func main() {\n")
	fmt.Fprintf(&out, "\tapp := astra.New(\n")
	fmt.Fprintf(&out, "\t\tastra.WithMode(astra.ModeProd),\n")
	fmt.Fprintf(&out, "\t\tastra.WithShutdownTimeout(30),\n")
	fmt.Fprintf(&out, "\t)\n\n")
	fmt.Fprintf(&out, "\tapp.Use(\n")
	fmt.Fprintf(&out, "\t\tmiddleware.RequestID(),\n")
	fmt.Fprintf(&out, "\t\tmiddleware.Logger(),\n")
	fmt.Fprintf(&out, "\t\tmiddleware.Recovery(),\n")
	fmt.Fprintf(&out, "\t\tmiddleware.CORS(\"https://example.com\"), // TODO: replace with your actual allowed origins\n")
	fmt.Fprintf(&out, "\t)\n\n")
	fmt.Fprintf(&out, "\tapp.GET(\"/health/live\", func(c *astra.Context) error {\n")
	fmt.Fprintf(&out, "\t\treturn c.JSON(http.StatusOK, astra.Map{\"status\": \"ok\"})\n")
	fmt.Fprintf(&out, "\t})\n")
	fmt.Fprintf(&out, "\tapp.GET(\"/health/ready\", func(c *astra.Context) error {\n")
	fmt.Fprintf(&out, "\t\treturn c.JSON(http.StatusOK, astra.Map{\"status\": \"ok\"})\n")
	fmt.Fprintf(&out, "\t})\n\n")
	fmt.Fprintf(&out, "\tv1 := app.Group(\"/api/v1\")\n")

	for _, svc := range services {
		implName    := svc.Name + "Impl"
		handlerName := svc.Name + "HTTPHandler"
		varBase     := strings.ToLower(svc.Name[:1]) + svc.Name[1:]
		fmt.Fprintf(&out, "\n")
		fmt.Fprintf(&out, "\t%sSvc := handler.New%s()\n", varBase, implName)
		fmt.Fprintf(&out, "\t%sH   := handler.New%s(%sSvc)\n", varBase, handlerName, varBase)
		fmt.Fprintf(&out, "\t%sH.Register(v1)\n", varBase)
	}
	if !hasServices {
		fmt.Fprintf(&out, "\t_ = v1 // TODO: wire handlers here\n")
	}

	fmt.Fprintf(&out, "\n\tapp.Run(\":8080\")\n")
	fmt.Fprintf(&out, "}\n")
	return out.String()
}

// buildTestFile generates _handler_test.go with one test function per RPC method.
func buildTestFile(module, base string, result genproto.ParseResult) string {
	var out strings.Builder

	fmt.Fprintf(&out, "// Code generated from %s.proto by astractl gen service --proto. DO NOT EDIT.\n\n", base)
	fmt.Fprintf(&out, "package handler_test\n\n")
	fmt.Fprintf(&out, "import (\n")
	fmt.Fprintf(&out, "\t\"net/http\"\n")
	fmt.Fprintf(&out, "\t\"net/http/httptest\"\n")
	fmt.Fprintf(&out, "\t\"testing\"\n\n")
	fmt.Fprintf(&out, "\t\"github.com/astra-go/astra\"\n")
	fmt.Fprintf(&out, "\t%q\n", module+"/internal/handler")
	fmt.Fprintf(&out, ")\n\n")

	for _, svc := range result.Services {
		implName    := svc.Name + "Impl"
		handlerName := svc.Name + "HTTPHandler"

		fmt.Fprintf(&out, "func setup%sApp(t *testing.T) *astra.App {\n", svc.Name)
		fmt.Fprintf(&out, "\tt.Helper()\n")
		fmt.Fprintf(&out, "\tapp := astra.New()\n")
		fmt.Fprintf(&out, "\tsvc := handler.New%s()\n", implName)
		fmt.Fprintf(&out, "\th := handler.New%s(svc)\n", handlerName)
		fmt.Fprintf(&out, "\th.Register(app.Group(\"/api/v1\"))\n")
		fmt.Fprintf(&out, "\treturn app\n")
		fmt.Fprintf(&out, "}\n\n")

		for _, rpc := range svc.RPCs {
			verb := rpc.HTTPVerb
			if verb == "" {
				verb = "POST"
			}
			verbTitle := strings.ToUpper(verb[:1]) + strings.ToLower(verb[1:])
			path := "/api/v1" + rpc.HTTPPath
			fmt.Fprintf(&out, "func Test%s_%s(t *testing.T) {\n", svc.Name, rpc.GoName)
			fmt.Fprintf(&out, "\tapp := setup%sApp(t)\n", svc.Name)
			fmt.Fprintf(&out, "\treq := httptest.NewRequest(http.Method%s, %q, nil)\n", verbTitle, path)
			fmt.Fprintf(&out, "\trec := httptest.NewRecorder()\n")
			fmt.Fprintf(&out, "\tapp.ServeHTTP(rec, req)\n")
			fmt.Fprintf(&out, "\t// TODO: assert expected status and body\n")
			fmt.Fprintf(&out, "\t_ = rec\n")
			fmt.Fprintf(&out, "}\n\n")
		}
	}
	return out.String()
}
