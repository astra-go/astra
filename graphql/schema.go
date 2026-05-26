package graphql

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/graphql-go/graphql"
)

// SchemaBuilder provides a fluent API to build GraphQL schemas from Go types.
type SchemaBuilder struct {
	queryType    *graphql.Object
	mutationType *graphql.Object
	subType      *graphql.Object
	description  string
}

// NewSchemaBuilder returns a fresh builder.
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{}
}

// WithDescription sets the schema description.
func (b *SchemaBuilder) WithDescription(s string) *SchemaBuilder {
	b.description = s
	return b
}

// WithQuery sets the root query object.
func (b *SchemaBuilder) WithQuery(t *graphql.Object) *SchemaBuilder {
	b.queryType = t
	return b
}

// WithMutation sets the root mutation object.
func (b *SchemaBuilder) WithMutation(t *graphql.Object) *SchemaBuilder {
	b.mutationType = t
	return b
}

// WithSubscription sets the root subscription object.
func (b *SchemaBuilder) WithSubscription(t *graphql.Object) *SchemaBuilder {
	b.subType = t
	return b
}

// Build creates the graphql.Schema from the current configuration.
func (b *SchemaBuilder) Build() (graphql.Schema, error) {
	if b.queryType == nil {
		return graphql.Schema{}, fmt.Errorf("graphql: schema requires at least a Query type")
	}
	cfg := graphql.SchemaConfig{}
	if b.queryType != nil {
		cfg.Query = b.queryType
	}
	if b.mutationType != nil {
		cfg.Mutation = b.mutationType
	}
	if b.subType != nil {
		cfg.Subscription = b.subType
	}
	return graphql.NewSchema(cfg)
}

// ─── Resolver registry ────────────────────────────────────────────────────────

// ResolverRegistry maps (TypeName, FieldName) → resolver func.
// Embed this interface in a struct to expose per-field resolvers.
type ResolverRegistry interface {
	// Resolve is called for each field that has a custom resolver.
	// Return nil to fall back to the default field-walk behavior.
	Resolve(ctx interface{}, p graphql.ResolveParams) (interface{}, error)
}

// FuncResolver adapts a plain function to a resolver.
type FuncResolver func(graphql.ResolveParams) (interface{}, error)

func (f FuncResolver) Resolve(_ interface{}, p graphql.ResolveParams) (interface{}, error) {
	return f(p)
}

// SimpleResolver maps "TypeName.FieldName" → FuncResolver.
// Usage:
//
//	resolver := graphql.SimpleResolver{
//	    "User.name": func(p graphql.ResolveParams) (interface{}, error) {
//	        user := p.Source.(*User)
//	        return user.FullName(), nil
//	    },
//	}
type SimpleResolver map[string]FuncResolver

func (r SimpleResolver) Resolve(_ interface{}, p graphql.ResolveParams) (interface{}, error) {
	typeName := ""
	if p.Info.ParentType != nil {
		typeName = p.Info.ParentType.Name()
	}
	key := typeName + "." + p.Info.FieldName
	if fn, ok := r[key]; ok {
		return fn(p)
	}
	return nil, nil
}

// ─── Go struct → GraphQL type mapper ─────────────────────────────────────────

// Tag "astra" struct fields:
//
//	astra:"name:FieldName"        → GraphQL field name (default: Go field name)
//	astra:"type:String!"          → explicit GraphQL type (non-null, list, etc.)
//	astra:"desc:description text" → field description
//	astra:"deprecated:reason"    → deprecation reason
//	astra:"arg:name:Int!"         → field argument (can be repeated for multiple args)
//
// Example:
//
//	type User struct {
//	    ID    int64  `astra:"type:ID!"`
//	    Name  string `astra:"name:firstName"`
//	    Email string `astra:"deprecated:use contactInfo"`
//	}
//
// MapStruct maps a Go struct to a graphql.Object.
// Each field gets a Resolve function that first checks RootValue["_resolver"]
// for a ResolverRegistry (set by MountSchema/NewHandler), then falls back to
// reading the matching struct field from p.Source.
func MapStruct(v interface{}, opts ...StructOption) *graphql.Object {
	cfg := structConfig{forType: reflect.TypeOf(v)}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return mapStructImpl(reflect.TypeOf(v), cfg)
}

type structConfig struct {
	forType   reflect.Type
	tagPrefix string
}

// StructOption configures the struct mapper.
type StructOption interface{ apply(*structConfig) }

type structOptFunc func(*structConfig)

func (f structOptFunc) apply(c *structConfig) { f(c) }

// WithTagPrefix uses a custom struct tag key (default: "astra").
func WithTagPrefix(prefix string) StructOption {
	return structOptFunc(func(c *structConfig) { c.tagPrefix = prefix })
}

func mapStructImpl(t reflect.Type, cfg structConfig) *graphql.Object {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	typeName := t.Name()
	fields := graphql.Fields{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := extractTagFieldName(f, cfg.tagPrefix)
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		gqlType, args := extractFieldTypeAndArgs(f, cfg.tagPrefix)
		// Capture loop variable for the closure.
		goFieldName := f.Name
		field := graphql.Field{
			Type:        gqlType,
			Description: extractTagFieldDesc(f, cfg.tagPrefix),
			Args:        args,
			Resolve:     makeFieldResolver(goFieldName),
		}
		if reason := extractTagFieldDeprecated(f, cfg.tagPrefix); reason != "" {
			field.DeprecationReason = reason
		}
		fields[name] = &field
	}
	return graphql.NewObject(graphql.ObjectConfig{
		Name:        typeName,
		Description: "",
		Fields:      fields,
	})
}

// makeFieldResolver returns a graphql.FieldResolveFn that:
//  1. Checks p.Info.RootValue["_resolver"] for a ResolverRegistry and calls it.
//  2. Falls back to reading the Go struct field from p.Source.
func makeFieldResolver(goFieldName string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		// Try ResolverRegistry from RootValue first.
		if rv, ok := p.Info.RootValue.(map[string]interface{}); ok {
			if reg, ok := rv["_resolver"].(ResolverRegistry); ok {
				val, err := reg.Resolve(nil, p)
				if val != nil || err != nil {
					return val, err
				}
			}
		}
		// Default: reflect the matching Go struct field from Source.
		return defaultFieldResolver(p.Source, goFieldName), nil
	}
}

// defaultFieldResolver reads goFieldName from src via reflection.
// Returns nil if src is nil or the field is not found.
func defaultFieldResolver(src interface{}, goFieldName string) interface{} {
	if src == nil {
		return nil
	}
	v := reflect.ValueOf(src)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName(goFieldName)
	if !f.IsValid() {
		return nil
	}
	return f.Interface()
}

// extractTagFieldName parses "name:FieldName" from the astra tag.
// Returns the Go field name if the tag is "-" (skip), empty string if the tag
// lacks a name: prefix (caller uses Go field name), or the explicit name.
func extractTagFieldName(f reflect.StructField, prefix string) string {
	if prefix == "" {
		prefix = "astra"
	}
	tag := f.Tag.Get(prefix)
	if tag == "" {
		return ""
	}
	first := strings.SplitN(tag, ",", 2)[0]
	if first == "" {
		return ""
	}
	if first == "-" {
		return "-"
	}
	if strings.HasPrefix(first, "name:") {
		return strings.TrimPrefix(first, "name:")
	}
	// Tag has type:/arg:/desc:/deprecated: but no name: → caller uses Go field name.
	return ""
}

func extractTagFieldDesc(f reflect.StructField, prefix string) string {
	if prefix == "" {
		prefix = "astra"
	}
	tag := f.Tag.Get(prefix)
	if tag == "" {
		return ""
	}
	for _, part := range strings.Split(tag, ",") {
		if strings.HasPrefix(part, "desc:") {
			return strings.TrimPrefix(part, "desc:")
		}
	}
	return ""
}

func extractTagFieldDeprecated(f reflect.StructField, prefix string) string {
	if prefix == "" {
		prefix = "astra"
	}
	tag := f.Tag.Get(prefix)
	if tag == "" {
		return ""
	}
	for _, part := range strings.Split(tag, ",") {
		if strings.HasPrefix(part, "deprecated:") {
			return strings.TrimPrefix(part, "deprecated:")
		}
	}
	return ""
}

// parseTypeString converts "String!", "[Int!]!", "ID!", etc. to a graphql.Output.
func parseTypeString(s string) graphql.Output {
	s = strings.TrimSpace(s)
	if s == "" {
		return graphql.String
	}
	nonNull := strings.HasSuffix(s, "!")
	list := strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")

	base := s
	if nonNull {
		base = strings.TrimSuffix(base, "!")
	}
	if list {
		base = strings.TrimPrefix(strings.TrimSuffix(base, "]"), "[")
	}

	gqlBase := scalarFromName(base)
	if list {
		if nonNull {
			return graphql.NewNonNull(graphql.NewList(gqlBase))
		}
		return graphql.NewList(gqlBase)
	}
	if nonNull {
		return graphql.NewNonNull(gqlBase)
	}
	return gqlBase
}

func scalarFromName(name string) graphql.Output {
	switch strings.ToLower(name) {
	case "id":
		return graphql.ID
	case "int", "int64":
		return graphql.Int
	case "float", "float64":
		return graphql.Float
	case "bool", "boolean":
		return graphql.Boolean
	case "string":
		return graphql.String
	default:
		// Unknown scalar → default to String (could be an enum)
		return graphql.String
	}
}

// goTypeToGQL maps a reflect.Type to a graphql.Output.
func goTypeToGQL(t reflect.Type) graphql.Output {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return graphql.Boolean
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return graphql.Int
	case reflect.Float32, reflect.Float64:
		return graphql.Float
	case reflect.String:
		return graphql.String
	case reflect.Slice:
		return graphql.NewList(goTypeToGQL(t.Elem()))
	default:
		return graphql.String
	}
}

// extractFieldTypeAndArgs parses type: and arg: tags from the struct field.
func extractFieldTypeAndArgs(f reflect.StructField, prefix string) (graphql.Output, graphql.FieldConfigArgument) {
	if prefix == "" {
		prefix = "astra"
	}
	tag := f.Tag.Get(prefix)
	var explicitType string
	var args graphql.FieldConfigArgument

	for _, part := range strings.Split(tag, ",") {
		if strings.HasPrefix(part, "type:") {
			explicitType = strings.TrimPrefix(part, "type:")
		}
		if strings.HasPrefix(part, "arg:") {
			argDef := strings.TrimPrefix(part, "arg:")
			name, parsed := parseArgDef(argDef)
			if parsed != nil {
				if args == nil {
					args = graphql.FieldConfigArgument{}
				}
				args[name] = parsed
			}
		}
	}

	var gqlType graphql.Output
	if explicitType != "" {
		gqlType = parseTypeString(explicitType)
	} else {
		gqlType = goTypeToGQL(f.Type)
	}
	return gqlType, args
}

func parseArgDef(def string) (name string, cfg *graphql.ArgumentConfig) {
	parts := strings.SplitN(def, ":", 2)
	if len(parts) < 2 {
		return "", nil
	}
	return parts[0], &graphql.ArgumentConfig{
		Type: parseTypeString(parts[1]),
	}
}

// MustBuildSchema is like Build but panics on error.
func MustBuildSchema(builder *SchemaBuilder) graphql.Schema {
	s, err := builder.Build()
	if err != nil {
		panic(fmt.Sprintf("graphql: schema build failed: %v", err))
	}
	return s
}
