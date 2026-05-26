// Package schema implements "astractl gen schema": scans Go source files and
// emits a native OpenAPI 3.1 specification (JSON or YAML).
//
// Annotation syntax (Go doc comments on handler functions):
//
//	// @summary  Create a user
//	// @desc     Creates a new user account and returns the created resource.
//	// @tags     users
//	// @param    body  body  CreateUserReq  true  "request body"
//	// @param    id    path  int            true  "user ID"
//	// @param    q     query string         false "search query"
//	// @success  201  {object}  User
//	// @failure  400  {object}  ErrorResponse
//	// @router   POST /users
//
// Struct fields are mapped to JSON Schema via their `json` and `validate` tags.
// Required fields are inferred from `validate:"required"`.
package schema

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astra-go/astra/cmd/astractl/internal/cli"
	"github.com/astra-go/astra/cmd/astractl/internal/fsutil"
)

// ─── CLI entry point ──────────────────────────────────────────────────────────

func Run(args []string) error {
	fs := flag.NewFlagSet("gen schema", flag.ContinueOnError)
	dir    := fs.String("dir",    ".",          "directory to scan (default: current directory)")
	out    := fs.String("out",    "openapi.json", "output file (use .yaml/.yml for YAML output)")
	title  := fs.String("title",  "API",         "OpenAPI info.title")
	ver    := fs.String("version","0.1.0",       "OpenAPI info.version")
	force  := fs.Bool("force",   false,          "overwrite existing output file")
	if err := fs.Parse(args); err != nil {
		return &cli.CLIError{Msg: "invalid flags: " + err.Error()}
	}

	spec, err := Generate(*dir, *title, *ver)
	if err != nil {
		return err
	}

	outFile := *out
	var raw []byte
	if strings.HasSuffix(outFile, ".yaml") || strings.HasSuffix(outFile, ".yml") {
		raw, err = yaml.Marshal(spec)
	} else {
		raw, err = json.MarshalIndent(spec, "", "  ")
	}
	if err != nil {
		return &cli.CLIError{Msg: "marshal spec: " + err.Error()}
	}

	outDir := filepath.Dir(outFile)
	outBase := filepath.Base(outFile)
	if outDir == "." {
		outDir = ""
	}
	if err := fsutil.WriteString(outDir, outBase, string(raw)+"\n", *force); err != nil {
		return err
	}

	schemaCount := len(spec.Components.Schemas)
	pathCount := len(spec.Paths)
	fmt.Printf("OpenAPI 3.1 spec generated: %s\n", outFile)
	fmt.Printf("  schemas: %d   paths: %d\n", schemaCount, pathCount)
	return nil
}

// ─── OpenAPI 3.1 data model ───────────────────────────────────────────────────

type Spec struct {
	OpenAPI    string              `json:"openapi" yaml:"openapi"`
	Info       Info                `json:"info" yaml:"info"`
	Paths      map[string]PathItem `json:"paths" yaml:"paths"`
	Components Components          `json:"components,omitempty" yaml:"components,omitempty"`
}

type Info struct {
	Title   string `json:"title" yaml:"title"`
	Version string `json:"version" yaml:"version"`
}

type Components struct {
	Schemas map[string]*Schema `json:"schemas,omitempty" yaml:"schemas,omitempty"`
}

type PathItem map[string]*Operation // key: "get","post","put","patch","delete","head","options"

type Operation struct {
	Summary     string              `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string              `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty" yaml:"tags,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses" yaml:"responses"`
	OperationID string              `json:"operationId,omitempty" yaml:"operationId,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name" yaml:"name"`
	In          string  `json:"in" yaml:"in"` // path|query|header|cookie
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool    `json:"required" yaml:"required"`
	Schema      *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

type RequestBody struct {
	Required bool                    `json:"required,omitempty" yaml:"required,omitempty"`
	Content  map[string]MediaType    `json:"content" yaml:"content"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

type Schema struct {
	Ref         string             `json:"$ref,omitempty" yaml:"$ref,omitempty"`
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Enum        []any              `json:"enum,omitempty" yaml:"enum,omitempty"`
	Minimum     *float64           `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum     *float64           `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	MinLength   *int               `json:"minLength,omitempty" yaml:"minLength,omitempty"`
	MaxLength   *int               `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	Pattern     string             `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Nullable    bool               `json:"nullable,omitempty" yaml:"nullable,omitempty"`
	Example     any                `json:"example,omitempty" yaml:"example,omitempty"`
}

// ─── Generator ────────────────────────────────────────────────────────────────

type generator struct {
	schemas map[string]*Schema
	paths   map[string]PathItem
	fset    *token.FileSet
}

// Generate scans all .go files under dir and returns an OpenAPI 3.1 Spec.
func Generate(dir, title, version string) (*Spec, error) {
	g := &generator{
		schemas: make(map[string]*Schema),
		paths:   make(map[string]PathItem),
		fset:    token.NewFileSet(),
	}

	pkgs, err := parser.ParseDir(g.fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, &cli.CLIError{
			Msg:  fmt.Sprintf("parse %s: %v", dir, err),
			Hint: "ensure the directory contains valid Go source files",
		}
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			g.scanFile(file)
		}
	}

	spec := &Spec{
		OpenAPI: "3.1.0",
		Info:    Info{Title: title, Version: version},
		Paths:   g.paths,
		Components: Components{
			Schemas: g.schemas,
		},
	}
	if len(spec.Paths) == 0 {
		spec.Paths = map[string]PathItem{}
	}
	return spec, nil
}

// scanFile processes a single parsed Go file.
func (g *generator) scanFile(file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			g.scanGenDecl(d)
		case *ast.FuncDecl:
			g.scanFuncDecl(d)
		}
	}
}

// ─── Struct → Schema ─────────────────────────────────────────────────────────

func (g *generator) scanGenDecl(decl *ast.GenDecl) {
	if decl.Tok != token.TYPE {
		return
	}
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}
		schema := g.structToSchema(st)
		if decl.Doc != nil {
			schema.Description = cleanComment(decl.Doc.Text())
		} else if ts.Comment != nil {
			schema.Description = cleanComment(ts.Comment.Text())
		}
		g.schemas[ts.Name.Name] = schema
	}
}

func (g *generator) structToSchema(st *ast.StructType) *Schema {
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}
	if st.Fields == nil {
		return schema
	}
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// Embedded field — skip for now.
			continue
		}
		jsonName, omitempty := jsonFieldName(field)
		if jsonName == "-" {
			continue
		}
		fieldSchema := g.exprToSchema(field.Type)
		applyValidateTags(fieldSchema, field.Tag)
		if field.Comment != nil {
			fieldSchema.Description = cleanComment(field.Comment.Text())
		} else if field.Doc != nil {
			fieldSchema.Description = cleanComment(field.Doc.Text())
		}
		schema.Properties[jsonName] = fieldSchema

		if isRequired(field.Tag) && !omitempty {
			schema.Required = append(schema.Required, jsonName)
		}
	}
	if len(schema.Required) > 0 {
		sort.Strings(schema.Required)
	}
	return schema
}

// jsonFieldName returns the JSON key and omitempty flag from the struct tag.
func jsonFieldName(field *ast.Field) (name string, omitempty bool) {
	if field.Tag == nil {
		if len(field.Names) > 0 {
			return field.Names[0].Name, false
		}
		return "", false
	}
	tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
	jsonTag := tag.Get("json")
	if jsonTag == "" {
		if len(field.Names) > 0 {
			return field.Names[0].Name, false
		}
		return "", false
	}
	parts := strings.Split(jsonTag, ",")
	n := parts[0]
	if n == "" && len(field.Names) > 0 {
		n = field.Names[0].Name
	}
	oo := len(parts) > 1 && parts[1] == "omitempty"
	return n, oo
}

// isRequired checks validate:"required" tag.
func isRequired(tag *ast.BasicLit) bool {
	if tag == nil {
		return false
	}
	t := reflect.StructTag(strings.Trim(tag.Value, "`"))
	v := t.Get("validate")
	for _, part := range strings.Split(v, ",") {
		if strings.TrimSpace(part) == "required" {
			return true
		}
	}
	return false
}

// applyValidateTags maps common validate constraints to JSON Schema keywords.
func applyValidateTags(s *Schema, tag *ast.BasicLit) {
	if tag == nil {
		return
	}
	t := reflect.StructTag(strings.Trim(tag.Value, "`"))
	v := t.Get("validate")
	if v == "" {
		return
	}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "min="):
			n, err := strconv.ParseFloat(part[4:], 64)
			if err == nil {
				if s.Type == "string" {
					i := int(n)
					s.MinLength = &i
				} else {
					s.Minimum = &n
				}
			}
		case strings.HasPrefix(part, "max="):
			n, err := strconv.ParseFloat(part[4:], 64)
			if err == nil {
				if s.Type == "string" {
					i := int(n)
					s.MaxLength = &i
				} else {
					s.Maximum = &n
				}
			}
		case strings.HasPrefix(part, "oneof="):
			vals := strings.Fields(part[6:])
			for _, val := range vals {
				s.Enum = append(s.Enum, val)
			}
		}
	}
}

// exprToSchema converts a Go AST type expression to a JSON Schema node.
func (g *generator) exprToSchema(expr ast.Expr) *Schema {
	switch t := expr.(type) {
	case *ast.Ident:
		return identToSchema(t.Name)
	case *ast.StarExpr:
		s := g.exprToSchema(t.X)
		s.Nullable = true
		return s
	case *ast.ArrayType:
		return &Schema{
			Type:  "array",
			Items: g.exprToSchema(t.Elt),
		}
	case *ast.MapType:
		return &Schema{Type: "object"}
	case *ast.SelectorExpr:
		// e.g. time.Time
		if id, ok := t.X.(*ast.Ident); ok {
			return selectorToSchema(id.Name, t.Sel.Name)
		}
		return &Schema{Type: "object"}
	case *ast.InterfaceType:
		return &Schema{} // any
	case *ast.StructType:
		return g.structToSchema(t)
	}
	return &Schema{Type: "object"}
}

func identToSchema(name string) *Schema {
	switch name {
	case "string":
		return &Schema{Type: "string"}
	case "bool":
		return &Schema{Type: "boolean"}
	case "int", "int8", "int16", "int32":
		return &Schema{Type: "integer", Format: "int32"}
	case "int64", "uint64":
		return &Schema{Type: "integer", Format: "int64"}
	case "uint", "uint8", "uint16", "uint32":
		return &Schema{Type: "integer", Format: "int32"}
	case "float32":
		return &Schema{Type: "number", Format: "float"}
	case "float64":
		return &Schema{Type: "number", Format: "double"}
	case "byte":
		return &Schema{Type: "string", Format: "byte"}
	case "rune":
		return &Schema{Type: "string"}
	case "any", "interface{}":
		return &Schema{}
	default:
		// Named type — emit a $ref; the struct will be in components/schemas.
		return &Schema{Ref: "#/components/schemas/" + name}
	}
}

func selectorToSchema(pkg, name string) *Schema {
	switch pkg + "." + name {
	case "time.Time":
		return &Schema{Type: "string", Format: "date-time"}
	case "time.Duration":
		return &Schema{Type: "string"}
	case "uuid.UUID", "uuid.NullUUID":
		return &Schema{Type: "string", Format: "uuid"}
	case "decimal.Decimal", "decimal.NullDecimal":
		return &Schema{Type: "string", Format: "decimal"}
	case "sql.NullString":
		return &Schema{Type: "string", Nullable: true}
	case "sql.NullInt64", "sql.NullInt32":
		return &Schema{Type: "integer", Nullable: true}
	case "sql.NullFloat64":
		return &Schema{Type: "number", Nullable: true}
	case "sql.NullBool":
		return &Schema{Type: "boolean", Nullable: true}
	}
	return &Schema{Type: "object"}
}

// ─── Handler annotations → Paths ─────────────────────────────────────────────

func (g *generator) scanFuncDecl(decl *ast.FuncDecl) {
	if decl.Doc == nil {
		return
	}
	ann := parseAnnotations(decl.Doc)
	if ann == nil {
		return
	}
	method := strings.ToLower(ann.method)
	path := ann.path
	if method == "" || path == "" {
		return
	}

	op := &Operation{
		Summary:     ann.summary,
		Description: ann.desc,
		Tags:        ann.tags,
		Responses:   make(map[string]Response),
	}
	if decl.Name != nil {
		op.OperationID = decl.Name.Name
	}

	// Parameters.
	for _, p := range ann.params {
		if p.in == "body" {
			op.RequestBody = &RequestBody{
				Required: p.required,
				Content: map[string]MediaType{
					"application/json": {Schema: typeRefSchema(p.typeName)},
				},
			}
		} else {
			op.Parameters = append(op.Parameters, Parameter{
				Name:        p.name,
				In:          p.in,
				Description: p.desc,
				Required:    p.required || p.in == "path",
				Schema:      primitiveSchema(p.typeName),
			})
		}
	}

	// Responses.
	for _, r := range ann.responses {
		resp := Response{Description: r.desc}
		if r.typeName != "" && r.typeName != "-" {
			resp.Content = map[string]MediaType{
				"application/json": {Schema: typeRefSchema(r.typeName)},
			}
		}
		op.Responses[r.code] = resp
	}
	if len(op.Responses) == 0 {
		op.Responses["200"] = Response{Description: "OK"}
	}

	if g.paths[path] == nil {
		g.paths[path] = make(PathItem)
	}
	g.paths[path][method] = op
}

// typeRefSchema returns a $ref if the name looks like a struct, else a primitive.
func typeRefSchema(name string) *Schema {
	if name == "" || name == "string" || name == "int" || name == "bool" || name == "number" {
		return primitiveSchema(name)
	}
	// Unwrap {object} / {array} wrappers from swaggo-style annotations.
	if strings.HasPrefix(name, "{") {
		inner := strings.Trim(name, "{}")
		if inner == "object" || inner == "array" {
			return &Schema{Type: inner}
		}
		return &Schema{Ref: "#/components/schemas/" + inner}
	}
	return &Schema{Ref: "#/components/schemas/" + name}
}

func primitiveSchema(name string) *Schema {
	switch strings.ToLower(name) {
	case "int", "integer", "int32", "int64":
		return &Schema{Type: "integer"}
	case "float", "float32", "float64", "number":
		return &Schema{Type: "number"}
	case "bool", "boolean":
		return &Schema{Type: "boolean"}
	default:
		return &Schema{Type: "string"}
	}
}

// ─── Annotation parser ────────────────────────────────────────────────────────

type paramDef struct {
	name     string
	in       string // path|query|header|cookie|body
	typeName string
	required bool
	desc     string
}

type responseDef struct {
	code     string
	typeName string
	desc     string
}

type annotations struct {
	summary   string
	desc      string
	tags      []string
	params    []paramDef
	responses []responseDef
	method    string
	path      string
}

// parseAnnotations extracts @-prefixed annotations from a doc comment group.
// Returns nil if no @router annotation is found (non-annotated functions are skipped).
func parseAnnotations(doc *ast.CommentGroup) *annotations {
	if doc == nil {
		return nil
	}
	ann := &annotations{}
	hasRouter := false

	for _, c := range doc.List {
		line := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if !strings.HasPrefix(line, "@") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		switch strings.ToLower(parts[0]) {
		case "@summary":
			ann.summary = strings.Join(parts[1:], " ")
		case "@desc", "@description":
			ann.desc = strings.Join(parts[1:], " ")
		case "@tags":
			ann.tags = parts[1:]
		case "@router":
			// @router METHOD /path  OR  @router /path [METHOD]
			if len(parts) >= 3 {
				// @router POST /users
				ann.method = parts[1]
				ann.path = parts[2]
			} else if len(parts) == 2 {
				ann.path = parts[1]
				ann.method = "GET"
			}
			hasRouter = true
		case "@param":
			// @param name in type required "desc"
			// e.g.: @param id path int true "user ID"
			if len(parts) < 4 {
				continue
			}
			p := paramDef{
				name:     parts[1],
				in:       strings.ToLower(parts[2]),
				typeName: parts[3],
			}
			if len(parts) >= 5 {
				p.required = strings.ToLower(parts[4]) == "true"
			}
			if len(parts) >= 6 {
				p.desc = strings.Trim(strings.Join(parts[5:], " "), `"`)
			}
			ann.params = append(ann.params, p)
		case "@success":
			// @success 200 {object} User "description"
			// @success 200 User "description"
			if len(parts) < 2 {
				continue
			}
			r := responseDef{code: parts[1], desc: "Success"}
			r.typeName, r.desc = parseResponseTypeParts(parts[2:], "Success")
			ann.responses = append(ann.responses, r)
		case "@failure":
			// @failure 400 {object} ErrorResponse "bad request"
			if len(parts) < 2 {
				continue
			}
			r := responseDef{code: parts[1], desc: "Error"}
			r.typeName, r.desc = parseResponseTypeParts(parts[2:], "Error")
			ann.responses = append(ann.responses, r)
		}
	}

	if !hasRouter {
		return nil
	}
	return ann
}

// cleanComment strips trailing whitespace and newlines from a comment block.
func cleanComment(s string) string {
	return strings.TrimSpace(s)
}

// parseResponseTypeParts handles both:
//   {object} TypeName "desc"   (swaggo-style)
//   TypeName "desc"            (short style)
func parseResponseTypeParts(parts []string, defaultDesc string) (typeName, desc string) {
	desc = defaultDesc
	if len(parts) == 0 {
		return
	}
	idx := 0
	// Skip {object}/{array} wrapper — the real type is the next token.
	if strings.HasPrefix(parts[0], "{") && strings.HasSuffix(parts[0], "}") {
		idx = 1
	}
	if idx < len(parts) {
		typeName = parts[idx]
		idx++
	}
	if idx < len(parts) {
		desc = strings.Trim(strings.Join(parts[idx:], " "), `"`)
	}
	return
}
