package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustGenerate(t *testing.T, src, title, version string) *Spec {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	spec, err := Generate(dir, title, version)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return spec
}

func schema(t *testing.T, spec *Spec, name string) *Schema {
	t.Helper()
	s, ok := spec.Components.Schemas[name]
	if !ok {
		t.Fatalf("schema %q not found; available: %v", name, schemaNames(spec))
	}
	return s
}

func schemaNames(spec *Spec) []string {
	names := make([]string, 0, len(spec.Components.Schemas))
	for n := range spec.Components.Schemas {
		names = append(names, n)
	}
	return names
}

func prop(t *testing.T, s *Schema, field string) *Schema {
	t.Helper()
	p, ok := s.Properties[field]
	if !ok {
		t.Fatalf("property %q not found in schema", field)
	}
	return p
}

// ─── Spec metadata ────────────────────────────────────────────────────────────

func TestSpecMetadata(t *testing.T) {
	spec := mustGenerate(t, "package p\n", "My API", "2.0.0")
	if spec.OpenAPI != "3.1.0" {
		t.Errorf("openapi = %q, want 3.1.0", spec.OpenAPI)
	}
	if spec.Info.Title != "My API" {
		t.Errorf("title = %q, want My API", spec.Info.Title)
	}
	if spec.Info.Version != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", spec.Info.Version)
	}
}

// ─── Struct extraction ────────────────────────────────────────────────────────

func TestStructExtraction(t *testing.T) {
	src := `package p
// User is a user resource.
type User struct {
	ID   int64  ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "User")
	if s.Type != "object" {
		t.Errorf("type = %q, want object", s.Type)
	}
	if s.Description == "" {
		t.Error("expected description from doc comment")
	}
	idProp := prop(t, s, "id")
	if idProp.Type != "integer" || idProp.Format != "int64" {
		t.Errorf("id: type=%q format=%q, want integer/int64", idProp.Type, idProp.Format)
	}
	nameProp := prop(t, s, "name")
	if nameProp.Type != "string" {
		t.Errorf("name: type=%q, want string", nameProp.Type)
	}
}

func TestStructNoJsonTag(t *testing.T) {
	src := `package p
type Req struct {
	Name string
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Req")
	// Falls back to field name when no json tag.
	if _, ok := s.Properties["Name"]; !ok {
		t.Error("expected property Name (no json tag fallback)")
	}
}

func TestJsonDashSkipped(t *testing.T) {
	src := `package p
type Req struct {
	Internal string ` + "`json:\"-\"`" + `
	Name     string ` + "`json:\"name\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Req")
	if _, ok := s.Properties["Internal"]; ok {
		t.Error("json:\"-\" field should be skipped")
	}
	if _, ok := s.Properties["-"]; ok {
		t.Error("json:\"-\" field should not appear as \"-\"")
	}
	if _, ok := s.Properties["name"]; !ok {
		t.Error("name field should be present")
	}
}

// ─── validate tag mapping ─────────────────────────────────────────────────────

func TestValidateRequired(t *testing.T) {
	src := `package p
type Req struct {
	Name  string ` + "`json:\"name\"  validate:\"required\"`" + `
	Email string ` + "`json:\"email\" validate:\"required\"`" + `
	Age   int    ` + "`json:\"age,omitempty\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Req")
	req := map[string]bool{}
	for _, r := range s.Required {
		req[r] = true
	}
	if !req["name"] {
		t.Error("name should be required")
	}
	if !req["email"] {
		t.Error("email should be required")
	}
	if req["age"] {
		t.Error("age (omitempty) should not be required")
	}
}

func TestValidateMinMax(t *testing.T) {
	src := `package p
type Req struct {
	Age  int    ` + "`json:\"age\"  validate:\"min=0,max=150\"`" + `
	Name string ` + "`json:\"name\" validate:\"min=1,max=64\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Req")

	age := prop(t, s, "age")
	if age.Minimum == nil || *age.Minimum != 0 {
		t.Errorf("age minimum = %v, want 0", age.Minimum)
	}
	if age.Maximum == nil || *age.Maximum != 150 {
		t.Errorf("age maximum = %v, want 150", age.Maximum)
	}

	name := prop(t, s, "name")
	if name.MinLength == nil || *name.MinLength != 1 {
		t.Errorf("name minLength = %v, want 1", name.MinLength)
	}
	if name.MaxLength == nil || *name.MaxLength != 64 {
		t.Errorf("name maxLength = %v, want 64", name.MaxLength)
	}
}

func TestValidateOneof(t *testing.T) {
	src := `package p
type Req struct {
	Role string ` + "`json:\"role\" validate:\"oneof=admin user guest\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Req")
	role := prop(t, s, "role")
	if len(role.Enum) != 3 {
		t.Fatalf("enum len = %d, want 3", len(role.Enum))
	}
	want := map[any]bool{"admin": true, "user": true, "guest": true}
	for _, v := range role.Enum {
		if !want[v] {
			t.Errorf("unexpected enum value %v", v)
		}
	}
}

// ─── Type mapping ─────────────────────────────────────────────────────────────

func TestPrimitiveTypes(t *testing.T) {
	src := `package p
import "time"
type All struct {
	S   string  ` + "`json:\"s\"`" + `
	I   int     ` + "`json:\"i\"`" + `
	I32 int32   ` + "`json:\"i32\"`" + `
	I64 int64   ` + "`json:\"i64\"`" + `
	F32 float32 ` + "`json:\"f32\"`" + `
	F64 float64 ` + "`json:\"f64\"`" + `
	B   bool    ` + "`json:\"b\"`" + `
	T   time.Time ` + "`json:\"t\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "All")

	cases := []struct{ field, typ, format string }{
		{"s", "string", ""},
		{"i", "integer", "int32"},
		{"i32", "integer", "int32"},
		{"i64", "integer", "int64"},
		{"f32", "number", "float"},
		{"f64", "number", "double"},
		{"b", "boolean", ""},
		{"t", "string", "date-time"},
	}
	for _, c := range cases {
		p := prop(t, s, c.field)
		if p.Type != c.typ {
			t.Errorf("%s: type=%q, want %q", c.field, p.Type, c.typ)
		}
		if p.Format != c.format {
			t.Errorf("%s: format=%q, want %q", c.field, p.Format, c.format)
		}
	}
}

func TestPointerNullable(t *testing.T) {
	src := `package p
type Req struct {
	Name *string ` + "`json:\"name\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Req")
	name := prop(t, s, "name")
	if !name.Nullable {
		t.Error("pointer field should be nullable")
	}
	if name.Type != "string" {
		t.Errorf("type = %q, want string", name.Type)
	}
}

func TestSliceType(t *testing.T) {
	src := `package p
type Resp struct {
	Tags []string ` + "`json:\"tags\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "Resp")
	tags := prop(t, s, "tags")
	if tags.Type != "array" {
		t.Errorf("type = %q, want array", tags.Type)
	}
	if tags.Items == nil || tags.Items.Type != "string" {
		t.Errorf("items.type = %v, want string", tags.Items)
	}
}

func TestNamedStructRef(t *testing.T) {
	src := `package p
type Address struct { City string ` + "`json:\"city\"`" + ` }
type User struct {
	Addr Address ` + "`json:\"addr\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "0")
	s := schema(t, spec, "User")
	addr := prop(t, s, "addr")
	if addr.Ref != "#/components/schemas/Address" {
		t.Errorf("$ref = %q, want #/components/schemas/Address", addr.Ref)
	}
}

// ─── Annotation parsing → paths ───────────────────────────────────────────────

func TestRouterAnnotation(t *testing.T) {
	src := `package p
// CreateUser creates a user.
//
// @summary  Create user
// @tags     users
// @param    body body CreateUserReq true "request body"
// @success  201 {object} User "created"
// @failure  400 {object} ErrorResponse "bad request"
// @router   POST /users
func CreateUser() {}
`
	spec := mustGenerate(t, src, "T", "0")
	item, ok := spec.Paths["/users"]
	if !ok {
		t.Fatal("path /users not found")
	}
	op, ok := item["post"]
	if !ok {
		t.Fatal("POST /users not found")
	}
	if op.Summary != "Create user" {
		t.Errorf("summary = %q, want Create user", op.Summary)
	}
	if len(op.Tags) == 0 || op.Tags[0] != "users" {
		t.Errorf("tags = %v, want [users]", op.Tags)
	}
	if op.RequestBody == nil {
		t.Fatal("requestBody is nil")
	}
	mt := op.RequestBody.Content["application/json"]
	if mt.Schema == nil || mt.Schema.Ref != "#/components/schemas/CreateUserReq" {
		t.Errorf("requestBody schema = %v, want $ref CreateUserReq", mt.Schema)
	}
	resp201, ok := op.Responses["201"]
	if !ok {
		t.Fatal("response 201 not found")
	}
	if resp201.Description != "created" {
		t.Errorf("201 desc = %q, want created", resp201.Description)
	}
	respSchema := resp201.Content["application/json"].Schema
	if respSchema == nil || respSchema.Ref != "#/components/schemas/User" {
		t.Errorf("201 schema = %v, want $ref User", respSchema)
	}
	resp400, ok := op.Responses["400"]
	if !ok {
		t.Fatal("response 400 not found")
	}
	if resp400.Description != "bad request" {
		t.Errorf("400 desc = %q, want bad request", resp400.Description)
	}
}

func TestPathParams(t *testing.T) {
	src := `package p
// GetUser gets a user.
//
// @summary  Get user
// @param    id path int true "user ID"
// @param    q  query string false "search"
// @success  200 {object} User
// @router   GET /users/:id
func GetUser() {}
`
	spec := mustGenerate(t, src, "T", "0")
	item, ok := spec.Paths["/users/:id"]
	if !ok {
		t.Fatal("path /users/:id not found")
	}
	op := item["get"]
	if len(op.Parameters) != 2 {
		t.Fatalf("params len = %d, want 2", len(op.Parameters))
	}
	byName := map[string]Parameter{}
	for _, p := range op.Parameters {
		byName[p.Name] = p
	}
	id := byName["id"]
	if id.In != "path" || !id.Required {
		t.Errorf("id: in=%q required=%v, want path/true", id.In, id.Required)
	}
	q := byName["q"]
	if q.In != "query" || q.Required {
		t.Errorf("q: in=%q required=%v, want query/false", q.In, q.Required)
	}
}

func TestNoRouterAnnotationSkipped(t *testing.T) {
	src := `package p
// helper does nothing.
func helper() {}
`
	spec := mustGenerate(t, src, "T", "0")
	if len(spec.Paths) != 0 {
		t.Errorf("expected no paths, got %v", spec.Paths)
	}
}

func TestDefaultResponse(t *testing.T) {
	src := `package p
// Ping pings.
//
// @summary Ping
// @router  GET /ping
func Ping() {}
`
	spec := mustGenerate(t, src, "T", "0")
	op := spec.Paths["/ping"]["get"]
	if _, ok := op.Responses["200"]; !ok {
		t.Error("expected default 200 response when no @success/@failure")
	}
}

// ─── parseResponseTypeParts ───────────────────────────────────────────────────

func TestParseResponseTypeParts(t *testing.T) {
	cases := []struct {
		parts       []string
		defaultDesc string
		wantType    string
		wantDesc    string
	}{
		{[]string{"{object}", "User", `"created"`}, "Success", "User", "created"},
		{[]string{"{object}", "User"}, "Success", "User", "Success"},
		{[]string{"User", `"ok"`}, "Success", "User", "ok"},
		{[]string{"User"}, "Success", "User", "Success"},
		{[]string{}, "Error", "", "Error"},
		{[]string{"{object}", "ErrorResponse", `"bad`, `request"`}, "Error", "ErrorResponse", "bad request"},
	}
	for _, c := range cases {
		gotType, gotDesc := parseResponseTypeParts(c.parts, c.defaultDesc)
		if gotType != c.wantType {
			t.Errorf("parts=%v: type=%q, want %q", c.parts, gotType, c.wantType)
		}
		if gotDesc != c.wantDesc {
			t.Errorf("parts=%v: desc=%q, want %q", c.parts, gotDesc, c.wantDesc)
		}
	}
}

// ─── JSON output ──────────────────────────────────────────────────────────────

func TestJSONOutput(t *testing.T) {
	src := `package p
type Item struct {
	ID int64 ` + "`json:\"id\"`" + `
}
`
	spec := mustGenerate(t, src, "Shop API", "1.0.0")
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"openapi":"3.1.0"`) {
		t.Error("JSON output missing openapi field")
	}
	if !strings.Contains(string(raw), `"Item"`) {
		t.Error("JSON output missing Item schema")
	}
}

// ─── Run CLI ──────────────────────────────────────────────────────────────────

func TestRunJSON(t *testing.T) {
	src := `package p
type Foo struct { X int ` + "`json:\"x\"`" + ` }
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.json")
	if err := Run([]string{"--dir", dir, "--out", out, "--title", "T", "--version", "0", "--force"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var spec Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Errorf("openapi = %q", spec.OpenAPI)
	}
	if _, ok := spec.Components.Schemas["Foo"]; !ok {
		t.Error("Foo schema missing from output")
	}
}

func TestRunYAML(t *testing.T) {
	src := `package p
type Bar struct { Y string ` + "`json:\"y\"`" + ` }
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.yaml")
	if err := Run([]string{"--dir", dir, "--out", out, "--title", "T", "--version", "0", "--force"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(raw), "openapi: 3.1.0") {
		t.Errorf("YAML output missing openapi field:\n%s", raw)
	}
	if !strings.Contains(string(raw), "Bar:") {
		t.Errorf("YAML output missing Bar schema:\n%s", raw)
	}
}

func TestRunForceOverwrite(t *testing.T) {
	src := `package p
type X struct{}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.json")
	// First write.
	if err := Run([]string{"--dir", dir, "--out", out, "--force"}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	// Second write without --force should fail.
	if err := Run([]string{"--dir", dir, "--out", out}); err == nil {
		t.Error("expected error without --force on existing file")
	}
	// With --force should succeed.
	if err := Run([]string{"--dir", dir, "--out", out, "--force"}); err != nil {
		t.Fatalf("Run with --force: %v", err)
	}
}

// ─── identToSchema ────────────────────────────────────────────────────────────

func TestIdentToSchema_AllPrimitives(t *testing.T) {
	cases := []struct {
		name   string
		wantTy string
		wantFmt string
	}{
		{"string", "string", ""},
		{"bool", "boolean", ""},
		{"int", "integer", "int32"},
		{"int8", "integer", "int32"},
		{"int16", "integer", "int32"},
		{"int32", "integer", "int32"},
		{"int64", "integer", "int64"},
		{"uint64", "integer", "int64"},
		{"uint", "integer", "int32"},
		{"uint8", "integer", "int32"},
		{"uint16", "integer", "int32"},
		{"uint32", "integer", "int32"},
		{"float32", "number", "float"},
		{"float64", "number", "double"},
		{"byte", "string", "byte"},
		{"rune", "string", ""},
		{"any", "", ""},
		{"interface{}", "", ""},
		{"MyStruct", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := identToSchema(tc.name)
			if tc.name == "MyStruct" {
				if s.Ref == "" {
					t.Errorf("expected $ref for named type, got %+v", s)
				}
				return
			}
			if s.Type != tc.wantTy {
				t.Errorf("identToSchema(%q).Type = %q, want %q", tc.name, s.Type, tc.wantTy)
			}
			if s.Format != tc.wantFmt {
				t.Errorf("identToSchema(%q).Format = %q, want %q", tc.name, s.Format, tc.wantFmt)
			}
		})
	}
}

// ─── selectorToSchema ─────────────────────────────────────────────────────────

func TestSelectorToSchema_KnownTypes(t *testing.T) {
	cases := []struct {
		pkg, name string
		wantTy    string
		wantFmt   string
	}{
		{"time", "Time", "string", "date-time"},
		{"time", "Duration", "string", ""},
		{"uuid", "UUID", "string", "uuid"},
		{"uuid", "NullUUID", "string", "uuid"},
		{"decimal", "Decimal", "string", "decimal"},
		{"decimal", "NullDecimal", "string", "decimal"},
		{"sql", "NullString", "string", ""},
		{"sql", "NullInt64", "integer", ""},
		{"sql", "NullInt32", "integer", ""},
		{"sql", "NullFloat64", "number", ""},
		{"sql", "NullBool", "boolean", ""},
		{"unknown", "Type", "object", ""},
	}
	for _, tc := range cases {
		t.Run(tc.pkg+"."+tc.name, func(t *testing.T) {
			s := selectorToSchema(tc.pkg, tc.name)
			if s.Type != tc.wantTy {
				t.Errorf("selectorToSchema(%q,%q).Type = %q, want %q", tc.pkg, tc.name, s.Type, tc.wantTy)
			}
		})
	}
}

// ─── typeRefSchema ────────────────────────────────────────────────────────────

func TestTypeRefSchema(t *testing.T) {
	cases := []struct {
		input   string
		wantRef string
		wantTy  string
	}{
		{"", "string", ""},
		{"string", "string", ""},
		{"int", "integer", ""},
		{"bool", "boolean", ""},
		{"number", "number", ""},
		{"{object}", "object", ""},
		{"{array}", "array", ""},
		{"{MyModel}", "", "#/components/schemas/MyModel"},
		{"MyModel", "", "#/components/schemas/MyModel"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			s := typeRefSchema(tc.input)
			if tc.wantRef != "" && s.Type != tc.wantRef {
				t.Errorf("typeRefSchema(%q).Type = %q, want %q", tc.input, s.Type, tc.wantRef)
			}
			if tc.wantTy != "" && s.Ref != tc.wantTy {
				t.Errorf("typeRefSchema(%q).Ref = %q, want %q", tc.input, s.Ref, tc.wantTy)
			}
		})
	}
}

// ─── primitiveSchema ─────────────────────────────────────────────────────────

func TestPrimitiveSchema(t *testing.T) {
	cases := []struct{ input, want string }{
		{"int", "integer"},
		{"integer", "integer"},
		{"int32", "integer"},
		{"int64", "integer"},
		{"float", "number"},
		{"float32", "number"},
		{"float64", "number"},
		{"number", "number"},
		{"bool", "boolean"},
		{"boolean", "boolean"},
		{"string", "string"},
		{"unknown", "string"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			s := primitiveSchema(tc.input)
			if s.Type != tc.want {
				t.Errorf("primitiveSchema(%q).Type = %q, want %q", tc.input, s.Type, tc.want)
			}
		})
	}
}

// ─── jsonFieldName ────────────────────────────────────────────────────────────

func TestGenerate_SelectorTypes(t *testing.T) {
	src := `package p
import "time"
type Event struct {
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "date-time") {
		t.Errorf("expected date-time format for time.Time, got:\n%s", raw)
	}
}

func TestGenerate_PointerField(t *testing.T) {
	src := `package p
type Resp struct {
	Name *string ` + "`json:\"name\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "name") {
		t.Errorf("expected name field in schema, got:\n%s", raw)
	}
}

func TestGenerate_ArrayField(t *testing.T) {
	src := `package p
type List struct {
	Items []string ` + "`json:\"items\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "array") {
		t.Errorf("expected array type in schema, got:\n%s", raw)
	}
}

func TestGenerate_MapField(t *testing.T) {
	src := `package p
type Meta struct {
	Labels map[string]string ` + "`json:\"labels\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "object") {
		t.Errorf("expected object type for map field, got:\n%s", raw)
	}
}

func TestGenerate_InterfaceField(t *testing.T) {
	src := `package p
type Flex struct {
	Data interface{} ` + "`json:\"data\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "data") {
		t.Errorf("expected data field in schema, got:\n%s", raw)
	}
}

func TestGenerate_HandlerWithQueryAndPathParams(t *testing.T) {
	src := `package p
// GetUser returns a user.
// @summary Get user by ID
// @router /users/{id} [get]
// @param id path int true "User ID"
// @param filter query string false "Filter"
// @response 200 {User} "Success"
// @response 404 string "Not found"
func GetUser() {}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "/users/{id}") {
		t.Errorf("expected path in spec, got:\n%s", raw)
	}
	if !strings.Contains(string(raw), "filter") {
		t.Errorf("expected query param in spec, got:\n%s", raw)
	}
}

func TestGenerate_HandlerWithBodyParam(t *testing.T) {
	src := `package p
// CreateUser creates a user.
// @summary Create user
// @router /users [post]
// @param body body CreateUserReq true "Request body"
// @response 201 {User} "Created"
func CreateUser() {}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "requestBody") {
		t.Errorf("expected requestBody in spec, got:\n%s", raw)
	}
}

func TestGenerate_OmitemptyField(t *testing.T) {
	src := `package p
type Partial struct {
	Name string ` + "`json:\"name,omitempty\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "name") {
		t.Errorf("expected name field, got:\n%s", raw)
	}
}

func TestGenerate_FieldWithNoTag(t *testing.T) {
	src := `package p
type NoTag struct {
	Value string
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	// Field name should fall back to struct field name
	if !strings.Contains(string(raw), "Value") {
		t.Errorf("expected Value field (no tag fallback), got:\n%s", raw)
	}
}

func TestGenerate_NestedStruct(t *testing.T) {
	src := `package p
type Address struct {
	City string ` + "`json:\"city\"`" + `
}
type Person struct {
	Name    string  ` + "`json:\"name\"`" + `
	Address Address ` + "`json:\"address\"`" + `
}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "Address") {
		t.Errorf("expected Address in schema, got:\n%s", raw)
	}
}

func TestGenerate_HandlerWithTags(t *testing.T) {
	src := `package p
// ListItems lists items.
// @summary List items
// @tags items inventory
// @router /items [get]
// @response 200 {array} "Items"
func ListItems() {}
`
	spec := mustGenerate(t, src, "T", "1.0")
	raw, _ := json.Marshal(spec)
	if !strings.Contains(string(raw), "items") {
		t.Errorf("expected tags in spec, got:\n%s", raw)
	}
}
