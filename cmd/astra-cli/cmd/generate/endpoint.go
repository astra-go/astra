package generate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/astra-go/astra/cmd/astra-cli/internal/fsutil"
	"github.com/astra-go/astra/cmd/astra-cli/internal/tpldata"
	"github.com/astra-go/astra/cmd/astra-cli/internal/templates"
	"gopkg.in/yaml.v3"
)

// NewGenerateEndpointCmd creates the `astra-cli generate endpoint` command.
func NewGenerateEndpointCmd(opts CmdOptions) *cobra.Command {
	var (
		interactive bool
		optFile     string
		optDir      string
		optPkg      string
	)

	c := &cobra.Command{
		Use:   "endpoint [openapi-file]",
		Short: "Generate HTTP handlers from an OpenAPI/Swagger spec",
		Long: `Parses an OpenAPI 3.x YAML or JSON file and generates Astra handler stubs
for every operation. Handler methods include:
  - Signature: func (h *Handler) OperationName(c *astra.Ctx) error
  - Proper request DTOs with JSON tags
  - Response types derived from schema components
  - Route registration scaffold (Register method)

Examples:
  astra-cli generate endpoint api/openapi.yaml
  astra-cli generate endpoint api/openapi.yaml -o ./internal/handler -p handler
  astra-cli generate endpoint  # interactive mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var file string
			if interactive || len(args) == 0 {
				f, err := opts.PromptString("OpenAPI file path", "api/openapi.yaml")
				if err != nil {
					return fmt.Errorf("cancelled: %w", err)
				}
				file = f
			} else {
				file = args[0]
			}

			if file == "" {
				return errors.New("OpenAPI file path is required")
			}
			if !opts.FileExists(file) {
				return fmt.Errorf("file not found: %s", file)
			}

			pkg := optPkg
			if pkg == "" {
				if interactive {
					pkg, _ = opts.PromptString("Handler package name", "handler")
				} else {
					pkg = inferPkgFromDir(optDir)
				}
			}

			outDir := optDir
			if outDir == "" && opts.OutDir != "" {
				outDir = opts.OutDir
			}

			fmt.Printf("▶  Parsing OpenAPI spec: %s\n", file)

			spec, err := loadOpenAPISpec(file)
			if err != nil {
				return fmt.Errorf("parse spec: %w", err)
			}

			apiTitle, _ := spec["info"].(map[string]any)["title"].(string)

			operations, err := extractOperations(spec)
			if err != nil {
				return fmt.Errorf("extract operations: %w", err)
			}
			if len(operations) == 0 {
				return errors.New("no operations found in OpenAPI spec")
			}

			// Render all handlers
			content, err := templates.RenderEndpoint(operations, pkg, apiTitle)
			if err != nil {
				return fmt.Errorf("render endpoint template: %w", err)
			}

			filename := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + "_handler.go"
			if outDir != "" {
				opts.MkdirAll(outDir)
				filename = filepath.Join(outDir, filename)
			}

			if opts.FileExists(filename) && !opts.Force {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", filename)
			}
			if err := fsutil.WriteString(filename, content); err != nil {
				return fmt.Errorf("write %s: %w", filename, err)
			}

			byTag := make(map[string][]templates.OpDef)
			for _, op := range operations {
				byTag[op.Tag] = append(byTag[op.Tag], op)
			}
			fmt.Printf("\n✓ Endpoint handlers generated: %s\n", filename)
			fmt.Printf("  Operations: %d\n", len(operations))
			for tag, ops := range byTag {
				fmt.Printf("  - %s: %d\n", tag, len(ops))
			}
			return nil
		},
	}

	c.Flags().BoolVar(&interactive, "interactive", false, "run in interactive mode")
	c.Flags().StringVarP(&optFile, "file", "f", "", "OpenAPI file path")
	c.Flags().StringVarP(&optDir, "dir", "d", "", "output directory (default: current directory)")
	c.Flags().StringVarP(&optPkg, "pkg", "p", "", "Go package name (default: inferred from dir)")

	return c
}

// ─── OpenAPI parsing ───────────────────────────────────────────────────────────

func loadOpenAPISpec(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var spec map[string]any

	// Try JSON first, then YAML
	if err := json.Unmarshal(data, &spec); err != nil {
		if yamlErr := yaml.Unmarshal(data, &spec); yamlErr != nil {
			return nil, fmt.Errorf("parse as JSON: %v; parse as YAML: %w", err, yamlErr)
		}
	}
	return spec, nil
}

func extractOperations(spec map[string]any) ([]templates.OpDef, error) {
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("spec missing 'paths' section")
	}

	httpMethods := []string{"get", "post", "put", "patch", "delete", "head", "options"}
	var ops []templates.OpDef

	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, p := range pathKeys {
		pv := paths[p].(map[string]any)
		for _, method := range httpMethods {
			opRaw, ok := pv[method]
			if !ok {
				continue
			}
			op := opRaw.(map[string]any)

			tag := "default"
			if tags, ok := op["tags"].([]any); ok && len(tags) > 0 {
				if t, ok := tags[0].(string); ok {
					tag = pascal(t)
				}
			}

			funcName := makeFuncName(op, method, p)
			summary, _ := op["summary"].(string)

			var reqType, respType string
			if req := extractRequestBody(op); req != "" {
				reqType = req
			}
			if resp := extractResponseBody(op); resp != "" {
				respType = resp
			}

			ops = append(ops, templates.OpDef{
				Method:   strings.ToUpper(method),
				Path:     p,
				FuncName: funcName,
				Summary:  summary,
				Tag:      tag,
				Request:  reqType,
				Response: respType,
			})
		}
	}
	return ops, nil
}

func extractRequestBody(op map[string]any) string {
	reqBody, ok := op["requestBody"].(map[string]any)
	if !ok {
		return ""
	}
	ct, _ := reqBody["content"].(map[string]any)
	for _, v := range ct {
		if m, ok := v.(map[string]any); ok {
			if schema, ok := m["schema"].(map[string]any); ok {
				return schemaTypeToGo(schema, nil)
			}
		}
	}
	return ""
}

func extractResponseBody(op map[string]any) string {
	responses, ok := op["responses"].(map[string]any)
	if !ok {
		return ""
	}
	// Find first non-2xx response with a body, or fall back to 200
	for _, code := range []string{"200", "201", "default"} {
		if resp, ok := responses[code].(map[string]any); ok {
			ct, ok := resp["content"].(map[string]any)
			if !ok {
				continue
			}
			for _, v := range ct {
				if m, ok := v.(map[string]any); ok {
					if schema, ok := m["schema"].(map[string]any); ok {
						return schemaTypeToGo(schema, nil)
					}
				}
			}
		}
	}
	return ""
}

func makeFuncName(op map[string]any, method, path string) string {
	if id, ok := op["operationId"].(string); ok && id != "" {
		return pascal(id)
	}
	clean := regexp.MustCompile(`[{}]`).ReplaceAllString(path, "")
	clean = regexp.MustCompile(`[^a-zA-Z0-9/]+`).ReplaceAllString(clean, "_")
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' })
	if len(parts) > 0 && strings.ToLower(parts[0]) == strings.ToLower(method) {
		parts = parts[1:]
	}
	return pascal(strings.Join(parts, "_"))
}

func schemaTypeToGo(schema map[string]any, schemas map[string]any) string {
	if ref, ok := schema["$ref"].(string); ok {
		parts := strings.Split(ref, "/")
		name := parts[len(parts)-1]
		// If schemas map is available, expand inline
		if schemas != nil {
			if _, ok := schemas[name].(map[string]any); ok {
				return "map[string]any /* " + name + " */"
			}
		}
		return pascal(name)
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
		return "[]" + schemaTypeToGo(items, schemas)
	case "object":
		return "map[string]any"
	default:
		return "any"
	}
}

func inferPkgFromDir(dir string) string {
	if dir == "" {
		return "handler"
	}
	name := filepath.Base(dir)
	return strings.ToLower(name)
}

// Suppress unused import
var _ = tpldata.Data{}
