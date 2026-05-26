package openapi

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
	"github.com/astra-go/astra/cmd/astractl/internal/tpldata"
)

func Run(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return &cli.CLIError{
			Msg:     "missing required argument: <file.yaml>",
			Example: "astractl gen openapi api/openapi.yaml --dir ./internal/handler",
		}
	}
	yamlFile := args[0]

	fs := flag.NewFlagSet("gen openapi", flag.ContinueOnError)
	dir   := fs.String("dir",   "", "output directory (default: current directory)")
	pkg   := fs.String("pkg",   "handler", "Go package name")
	force := fs.Bool("force", false, "overwrite existing file")
	if err := fs.Parse(args[1:]); err != nil {
		return &cli.CLIError{Msg: "invalid flags: " + err.Error()}
	}

	raw, err := os.ReadFile(yamlFile)
	if err != nil {
		return &cli.CLIError{
			Msg:     fmt.Sprintf("read %s: %v", yamlFile, err),
			Hint:    "ensure the file path is correct and readable",
			Example: "astractl gen openapi api/openapi.yaml",
		}
	}

	var spec map[string]any
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("parse %s: %v", yamlFile, err),
			Hint: "ensure the file is valid YAML / OpenAPI 3.x",
		}
	}

	type opDef struct {
		Method   string
		Path     string
		FuncName string
		Comment  string
	}

	tagOps := make(map[string][]opDef)
	httpMethods := []string{"get", "post", "put", "patch", "delete", "head", "options"}

	paths, _ := spec["paths"].(map[string]any)
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, p := range pathKeys {
		pv := paths[p]
		pathItem, _ := pv.(map[string]any)
		for _, method := range httpMethods {
			opRaw, ok := pathItem[method]
			if !ok {
				continue
			}
			op, _ := opRaw.(map[string]any)

			funcName := ""
			if id, ok := op["operationId"].(string); ok && id != "" {
				funcName = tpldata.Pascal(id)
			} else {
				clean := regexp.MustCompile(`[{}]`).ReplaceAllString(p, "")
				clean = regexp.MustCompile(`[^a-zA-Z0-9/]+`).ReplaceAllString(clean, "_")
				parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' })
				funcName = tpldata.Pascal(strings.ToLower(method)) + tpldata.Pascal(strings.Join(parts, "_"))
			}

			comment := funcName
			if s, ok := op["summary"].(string); ok && s != "" {
				comment = s
			}

			tag := "default"
			if tags, ok := op["tags"].([]any); ok && len(tags) > 0 {
				if t, ok := tags[0].(string); ok && t != "" {
					tag = t
				}
			}

			tagOps[tag] = append(tagOps[tag], opDef{
				Method:   strings.ToUpper(method),
				Path:     p,
				FuncName: funcName,
				Comment:  comment,
			})
		}
	}

	if len(tagOps) == 0 {
		return &cli.CLIError{
			Msg:  fmt.Sprintf("no paths/operations found in %s", yamlFile),
			Hint: "ensure the file has a 'paths' section with at least one HTTP operation",
		}
	}

	tags := make([]string, 0, len(tagOps))
	for t := range tagOps {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var buf strings.Builder
	fmt.Fprintf(&buf, "package %s\n\n", *pkg)
	fmt.Fprintf(&buf, "import \"github.com/astra-go/astra\"\n\n")

	// Emit Go types from components/schemas.
	if schemaTypes := extractSchemas(spec); schemaTypes != "" {
		buf.WriteString(schemaTypes)
	}

	for _, tag := range tags {
		ops := tagOps[tag]
		handlerName := tpldata.Pascal(tag) + "Handler"

		fmt.Fprintf(&buf, "// %s handles %s API endpoints.\n", handlerName, tag)
		fmt.Fprintf(&buf, "type %s struct{}\n\n", handlerName)

		for _, op := range ops {
			fmt.Fprintf(&buf, "// %s handles %s %s.\n", op.FuncName, op.Method, op.Path)
			fmt.Fprintf(&buf, "func (h *%s) %s(c *astra.Context) error {\n", handlerName, op.FuncName)
			fmt.Fprintf(&buf, "\t// TODO: implement — %s\n", op.Comment)
			fmt.Fprintf(&buf, "\treturn nil\n}\n\n")
		}

		fmt.Fprintf(&buf, "// Register mounts the %s routes on the given router group.\n", tag)
		fmt.Fprintf(&buf, "func (h *%s) Register(g *astra.Group) {\n", handlerName)
		for _, op := range ops {
			fmt.Fprintf(&buf, "\tg.%s(%q, h.%s)\n", op.Method, op.Path, op.FuncName)
		}
		fmt.Fprintf(&buf, "}\n\n")
	}

	base := strings.TrimSuffix(filepath.Base(yamlFile), filepath.Ext(yamlFile))
	filename := base + "_handler.go"
	if err := fsutil.WriteString(*dir, filename, buf.String(), *force); err != nil {
		return err
	}

	if *dir != "" {
		fmt.Printf("OpenAPI handlers generated: %s/%s\n", *dir, filename)
	} else {
		fmt.Printf("OpenAPI handlers generated: %s\n", filename)
	}
	for _, tag := range tags {
		fmt.Printf("  %sHandler (%d operation(s))\n", tpldata.Pascal(tag), len(tagOps[tag]))
	}
	return nil
}

// extractSchemas generates Go struct definitions from components/schemas.
func extractSchemas(spec map[string]any) string {
	components, _ := spec["components"].(map[string]any)
	if components == nil {
		return ""
	}
	schemas, _ := components["schemas"].(map[string]any)
	if len(schemas) == 0 {
		return ""
	}

	names := make([]string, 0, len(schemas))
	for n := range schemas {
		names = append(names, n)
	}
	sort.Strings(names)

	var buf strings.Builder
	fmt.Fprintf(&buf, "// ─── Request / Response Types ───────────────────────────────────────────────\n\n")
	for _, name := range names {
		schema, ok := schemas[name].(map[string]any)
		if !ok {
			continue
		}
		genSchemaType(&buf, tpldata.Pascal(name), schema, schemas)
	}
	return buf.String()
}

// genSchemaType writes a single Go type definition for an OpenAPI schema object.
func genSchemaType(buf *strings.Builder, goName string, schema, allSchemas map[string]any) {
	typ, _ := schema["type"].(string)
	if typ != "" && typ != "object" {
		fmt.Fprintf(buf, "type %s = %s\n\n", goName, schemaToGoType(schema, allSchemas))
		return
	}

	props, _ := schema["properties"].(map[string]any)
	required := make(map[string]bool)
	if reqList, ok := schema["required"].([]any); ok {
		for _, r := range reqList {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}

	fmt.Fprintf(buf, "type %s struct {\n", goName)
	if len(props) > 0 {
		propNames := make([]string, 0, len(props))
		for p := range props {
			propNames = append(propNames, p)
		}
		sort.Strings(propNames)

		for _, pName := range propNames {
			pSchema, ok := props[pName].(map[string]any)
			if !ok {
				continue
			}
			goType := schemaToGoType(pSchema, allSchemas)
			jsonTag := pName
			if !required[pName] {
				jsonTag += ",omitempty"
			}
			fmt.Fprintf(buf, "\t%-20s %-20s `json:%q form:%q`\n",
				tpldata.Pascal(pName), goType, jsonTag, pName)
		}
	}
	fmt.Fprintf(buf, "}\n\n")
}

// schemaToGoType converts an OpenAPI schema node to a Go type string.
func schemaToGoType(schema, allSchemas map[string]any) string {
	if ref, ok := schema["$ref"].(string); ok {
		parts := strings.Split(ref, "/")
		return tpldata.Pascal(parts[len(parts)-1])
	}
	typ, _ := schema["type"].(string)
	format, _ := schema["format"].(string)
	switch typ {
	case "string":
		return "string"
	case "integer":
		if format == "int32" {
			return "int32"
		}
		return "int64"
	case "number":
		if format == "float" {
			return "float32"
		}
		return "float64"
	case "boolean":
		return "bool"
	case "array":
		items, _ := schema["items"].(map[string]any)
		if items == nil {
			return "[]any"
		}
		return "[]" + schemaToGoType(items, allSchemas)
	case "object":
		return "map[string]any"
	default:
		return "any"
	}
}
