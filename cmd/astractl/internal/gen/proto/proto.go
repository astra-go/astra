package proto

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
)

// Run implements the gen proto subcommand.
func Run(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return &cli.CLIError{
			Msg:     "missing required argument: <file.proto>",
			Example: "astractl gen proto api/service.proto --dir ./internal/handler --pkg handler",
		}
	}
	protoFile := args[0]

	fs := flag.NewFlagSet("gen proto", flag.ContinueOnError)
	dir      := fs.String("dir",      "",        "output directory (default: current directory)")
	pkg      := fs.String("pkg",      "handler", "Go package name")
	module   := fs.String("module",   "",        "Go module path for imports (default: github.com/astra-go/astra)")
	grpcOnly := fs.Bool("grpc",       false,     "pure gRPC-first: generate types + interface + gRPC registration stub only (no HTTP adapter)")
	contract := fs.Bool("contract",   false,     "generate only types + service interface (no HTTP adapter)")
	impl     := fs.Bool("impl",       false,     "also generate service implementation skeleton")
	force    := fs.Bool("force",      false,     "overwrite existing file(s)")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{
			Msg:     "invalid flags: " + err.Error(),
			Example: "astractl gen proto api/service.proto --pkg handler --force",
		}
	}

	raw, err := os.ReadFile(protoFile)
	if err != nil {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("read %s: %v", protoFile, err),
			Hint:    "ensure the .proto file path is correct and readable",
			Example: "astractl gen proto api/service.proto",
		}
	}

	src := StripComments(string(raw))
	skipHTTP := *grpcOnly || *contract
	result := Parse(src, skipHTTP)

	if len(result.Services) == 0 && len(result.Messages) == 0 {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("no service or message definitions found in %s", protoFile),
			Hint: "ensure the file contains at least one 'message' or 'service' block with valid proto3 syntax",
			Example: `syntax = "proto3";
message GetUserRequest { string id = 1; }
service UserService { rpc GetUser(GetUserRequest) returns (GetUserResponse); }`,
		}
	}

	base := strings.TrimSuffix(filepath.Base(protoFile), ".proto")
	protoBase := filepath.Base(protoFile)

	out := BuildOutput(protoBase, *pkg, *dir, *module, *grpcOnly, *contract, result)

	mainFile := base + "_handler.go"
	if *grpcOnly {
		mainFile = base + "_grpc.go"
	} else if *contract {
		mainFile = base + "_contract.go"
	}
	if err := fsutil.WriteString(*dir, mainFile, out, *force); err != nil {
		return err
	}

	printPath := mainFile
	if *dir != "" {
		printPath = *dir + "/" + mainFile
	}
	fmt.Printf("Proto generated: %s\n", printPath)
	withHTTP := !*contract && !*grpcOnly && len(result.Services) > 0
	for _, svc := range result.Services {
		fmt.Printf("  %sServer interface (%d method(s))\n", svc.Name, len(svc.RPCs))
		if withHTTP {
			fmt.Printf("  %sHTTPHandler adapter\n", svc.Name)
		} else if *grpcOnly {
			fmt.Printf("  gRPC registration stub (wire with grpc.RegisterXxxServer)\n")
		}
	}
	fmt.Printf("  %d message type(s), %d enum(s)\n", len(result.Messages), len(result.Enums))

	if *impl && len(result.Services) > 0 {
		ib := BuildImplSkeleton(*pkg, result.Services)
		implFile := base + "_impl.go"
		if err := fsutil.WriteString(*dir, implFile, ib, *force); err != nil {
			return err
		}
		implPath := implFile
		if *dir != "" {
			implPath = *dir + "/" + implFile
		}
		fmt.Printf("  Implementation skeleton: %s\n", implPath)
	}
	return nil
}

func BuildOutput(protoBase, pkg, dir, modulePath string, grpcOnly, contract bool, result ParseResult) string {
	var out strings.Builder
	withHTTP := !contract && !grpcOnly && len(result.Services) > 0

	if modulePath == "" {
		modulePath = "github.com/astra-go/astra"
	}
	// derive the import alias from the last path segment
	frameworkPkg := modulePath[strings.LastIndex(modulePath, "/")+1:]
	fmt.Fprintf(&out, "// Code generated from %s by astractl gen proto. DO NOT EDIT.\n", protoBase)
	fmt.Fprintf(&out, "// To regenerate: astractl gen proto %s", protoBase)
	if dir != "" {
		fmt.Fprintf(&out, " --dir %s", dir)
	}
	fmt.Fprintf(&out, " --pkg %s --force\n\n", pkg)
	fmt.Fprintf(&out, "package %s\n\n", pkg)

	var imports []string
	if len(result.Services) > 0 {
		imports = append(imports, `"context"`)
	}
	if withHTTP {
		imports = append(imports, `"net/http"`, `""`, fmt.Sprintf("%q", modulePath))
	}
	if len(imports) > 0 {
		fmt.Fprintf(&out, "import (\n")
		for _, imp := range imports {
			if imp == `""` {
				fmt.Fprintf(&out, "\n")
			} else {
				fmt.Fprintf(&out, "\t%s\n", imp)
			}
		}
		fmt.Fprintf(&out, ")\n\n")
	}

	if len(result.Enums) > 0 {
		fmt.Fprintf(&out, "// ─── Enums ───────────────────────────────────────────────────────────────────\n\n")
		for _, e := range result.Enums {
			fmt.Fprintf(&out, "// %s represents the proto enum %s.\n", e.Name, e.Name)
			fmt.Fprintf(&out, "type %s int32\n\nconst (\n", e.Name)
			for _, v := range e.Values {
				fmt.Fprintf(&out, "\t%s %s = %s\n", v.GoName, e.Name, v.Number)
			}
			fmt.Fprintf(&out, ")\n\n")
		}
	}

	if len(result.Messages) > 0 {
		fmt.Fprintf(&out, "// ─── Messages ────────────────────────────────────────────────────────────────\n\n")
		for _, m := range result.Messages {
			fmt.Fprintf(&out, "// %s is generated from the proto message %s.\n", m.Name, m.Name)
			fmt.Fprintf(&out, "type %s struct {\n", m.Name)
			for _, f := range m.Fields {
				fmt.Fprintf(&out, "\t%-20s %-20s `json:%q form:%q`\n",
					f.GoName, f.GoType, f.JSONTag, f.JSONTag)
			}
			fmt.Fprintf(&out, "}\n\n")
		}
	}

	for _, svc := range result.Services {
		ifaceName := svc.Name + "Server"
		handlerName := svc.Name + "HTTPHandler"

		fmt.Fprintf(&out, "// ─── %s ──────────────────────────────────────────────────────────────────────\n\n", svc.Name)
		fmt.Fprintf(&out, "// %s is the contract interface for %s.\n", ifaceName, svc.Name)
		fmt.Fprintf(&out, "// Implement this once in your service layer; the HTTP adapter below and any\n")
		fmt.Fprintf(&out, "// gRPC server both depend on it — implement once, expose over any transport.\n")
		fmt.Fprintf(&out, "type %s interface {\n", ifaceName)
		for _, rpc := range svc.RPCs {
			fmt.Fprintf(&out, "\t%s(ctx context.Context, req *%s) (*%s, error)\n", rpc.GoName, rpc.Req, rpc.Resp)
		}
		fmt.Fprintf(&out, "}\n\n")

		if contract {
			continue
		}

		if grpcOnly {
			fmt.Fprintf(&out, "// Register%s wires impl into a gRPC server.\n", svc.Name)
			fmt.Fprintf(&out, "func Register%s(s interface {\n", svc.Name)
			fmt.Fprintf(&out, "\tRegisterService(*ServiceDesc, interface{})\n")
			fmt.Fprintf(&out, "}, impl %s) {\n", ifaceName)
			fmt.Fprintf(&out, "\t// Replace with: pb.Register%sServer(s, impl)\n", svc.Name)
			fmt.Fprintf(&out, "\t_ = impl\n")
			fmt.Fprintf(&out, "}\n\n")
			continue
		}

		fmt.Fprintf(&out, "// %s wraps %s as Astra HTTP endpoints.\n", handlerName, ifaceName)
		fmt.Fprintf(&out, "//\n//\th := New%s(impl)\n", handlerName)
		fmt.Fprintf(&out, "//\th.Register(app.Group(\"/api/v1\"))\n")
		fmt.Fprintf(&out, "type %s struct{ svc %s }\n\n", handlerName, ifaceName)
		fmt.Fprintf(&out, "// New%s creates an HTTP adapter for %s.\n", handlerName, ifaceName)
		fmt.Fprintf(&out, "func New%s(svc %s) *%s { return &%s{svc: svc} }\n\n",
			handlerName, ifaceName, handlerName, handlerName)

		fmt.Fprintf(&out, "// Register mounts %s routes on g.\n", svc.Name)
		fmt.Fprintf(&out, "func (h *%s) Register(g *%s.Group) {\n", handlerName, frameworkPkg)
		for _, rpc := range svc.RPCs {
			fmt.Fprintf(&out, "\tg.%-7s(%q, h.%s)\n", rpc.HTTPVerb, rpc.HTTPPath, rpc.GoName)
		}
		fmt.Fprintf(&out, "}\n\n")

		for _, rpc := range svc.RPCs {
			bindFn := "ShouldBindQuery"
			if rpc.UseBody {
				bindFn = "ShouldBindJSON"
			}
			statusCode := "http.StatusOK"
			if rpc.HTTPVerb == "POST" {
				statusCode = "http.StatusCreated"
			}
			fmt.Fprintf(&out, "// %s handles %s %s.\n", rpc.GoName, rpc.HTTPVerb, rpc.HTTPPath)
			fmt.Fprintf(&out, "func (h *%s) %s(c *%s.Context) error {\n", handlerName, rpc.GoName, frameworkPkg)
			fmt.Fprintf(&out, "\tvar req %s\n", rpc.Req)
			fmt.Fprintf(&out, "\tif err := c.%s(&req); err != nil {\n\t\treturn err\n\t}\n", bindFn)
			fmt.Fprintf(&out, "\tresp, err := h.svc.%s(c.Request.Context(), &req)\n", rpc.GoName)
			fmt.Fprintf(&out, "\tif err != nil {\n\t\treturn err\n\t}\n")
			if rpc.HTTPVerb == "DELETE" {
				fmt.Fprintf(&out, "\t_ = resp\n\treturn c.NoContent(http.StatusNoContent)\n}\n\n")
			} else {
				fmt.Fprintf(&out, "\treturn c.JSON(%s, resp)\n}\n\n", statusCode)
			}
		}
	}

	return out.String()
}

func BuildImplSkeleton(pkg string, svcs []SvcInfo) string {
	var ib strings.Builder
	fmt.Fprintf(&ib, "// Service implementation skeleton — fill in each method.\n")
	fmt.Fprintf(&ib, "// This file is NOT overwritten by 'gen proto' (no --force needed here).\n\n")
	fmt.Fprintf(&ib, "package %s\n\nimport \"context\"\n\n", pkg)
	for _, svc := range svcs {
		ifaceName := svc.Name + "Server"
		implName := svc.Name + "Impl"
		fmt.Fprintf(&ib, "// %s implements %s.\n", implName, ifaceName)
		fmt.Fprintf(&ib, "type %s struct {\n\t// TODO: inject dependencies\n}\n\n", implName)
		fmt.Fprintf(&ib, "// New%s creates a new %s.\n", implName, implName)
		fmt.Fprintf(&ib, "func New%s() *%s { return &%s{} }\n\n", implName, implName, implName)
		for _, rpc := range svc.RPCs {
			fmt.Fprintf(&ib, "func (s *%s) %s(ctx context.Context, req *%s) (*%s, error) {\n",
				implName, rpc.GoName, rpc.Req, rpc.Resp)
			fmt.Fprintf(&ib, "\t_ = ctx\n\t// TODO: implement\n\treturn nil, nil\n}\n\n")
		}
	}
	return ib.String()
}
