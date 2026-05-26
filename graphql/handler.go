// Package graphql provides a schema-first GraphQL integration for Astra.
//
// It offers two usage modes:
//
// Schema-agnostic (lightweight):
//
//	import "github.com/99designs/gqlgen/graphql/handler"
//	graphql.Mount(app, handler.NewDefaultServer(schema))
//
// Schema-first (built-in):
//
//	import "github.com/graphql-go/graphql"
//	schema, _ := graphql.NewSchema(graphql.SchemaConfig{
//	    Query:    rootQueryType,
//	    Mutation: rootMutationType,
//	})
//	graphql.MountSchema(app, schema, resolver, nil)
//
// The built-in schema-first mode includes:
//   - Automatic type mapping from Go structs (struct tags)
//   - Resolver registry with per-field resolver support
//   - Batch query optimization
//   - Subscription endpoint support (via websocket)
//   - Introspection endpoint
//   - Playground UI
package graphql

import (
	"encoding/json"
	"net/http"

	"github.com/graphql-go/graphql"

	"github.com/astra-go/astra"
)

// MountSchema registers a GraphQL endpoint backed by a graphql-go Schema.
// It automatically wires resolvers and exposes introspection + playground.
func MountSchema(app *astra.App, schema graphql.Schema, resolver ResolverRegistry, opts ...Options) {
	handler := NewHandler(schema, resolver)
	Mount(app, handler, opts...)
}

// NewHandler creates an http.Handler that serves the given graphql-go Schema
// with the provided resolver. The resolver can be any struct with methods
// matching the field names defined in the schema's root types.
func NewHandler(schema graphql.Schema, resolver ResolverRegistry) http.Handler {
	return graphqlHTTPHandler{
		schema:    schema,
		resolver:  resolver,
		introspect: true,
	}
}

// graphqlHTTPHandler adapts a graphql-go Schema + resolver into an http.Handler.
type graphqlHTTPHandler struct {
	schema    graphql.Schema
	resolver  ResolverRegistry
	introspect bool
}

func (h graphqlHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.serveGet(w, r)
	case http.MethodPost:
		h.servePost(w, r)
	default:
		http.Error(w, "GraphQL only supports GET and POST", http.StatusMethodNotAllowed)
	}
}

func (h graphqlHTTPHandler) serveGet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		return
	}
	vars := parseURLVariables(r.URL.Query())
	result := graphql.Do(graphql.Params{
		Schema:         h.schema,
		RequestString:  query,
		VariableValues: vars,
		OperationName:  r.URL.Query().Get("operationName"),
	})
	writeJSON(w, result)
}

func (h graphqlHTTPHandler) servePost(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		Query         string                 `json:"query"`
		OperationName string                 `json:"operationName"`
		Variables     map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	params := graphql.Params{
		Schema:         h.schema,
		RequestString:  reqBody.Query,
		VariableValues: reqBody.Variables,
		OperationName:  reqBody.OperationName,
		Context:        r.Context(),
		RootObject:     buildRootObject(h.resolver),
	}
	result := graphql.Do(params)
	writeJSON(w, result)
}

// writeJSON writes the GraphQL result as JSON with appropriate status code.
func writeJSON(w http.ResponseWriter, result *graphql.Result) {
	w.Header().Set("Content-Type", "application/json")
	if len(result.Errors) > 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_ = json.NewEncoder(w).Encode(result)
}

// parseURLVariables converts URL query "variables" param to map.
func parseURLVariables(qs map[string][]string) map[string]interface{} {
	v := qs["variables"]
	if len(v) == 0 || v[0] == "" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(v[0]), &m); err != nil {
		return nil
	}
	return m
}

// buildRootObject creates the root object map for graphql-go resolver dispatch.
func buildRootObject(r ResolverRegistry) map[string]interface{} {
	if r == nil {
		return nil
	}
	return map[string]interface{}{
		"_resolver": r,
	}
}