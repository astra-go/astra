package graphql_test

import (
	"net/http"
	"testing"

	"github.com/graphql-go/graphql"

	astragraphql "github.com/astra-go/astra/graphql"
	"github.com/astra-go/astra/testutil"
)

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ─── NewHandler ────────────────────────────────────────────────────────────────

func TestNewHandler_POSTQuery(t *testing.T) {
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"hello": &graphql.Field{
				Type: graphql.String,
				Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
					return "world", nil
				},
			},
		},
	})
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, nil)
	s := testutil.NewServer(t, app)

	resp := s.POST("/graphql", map[string]string{"query": "{ hello }"})
	resp.AssertStatus(http.StatusOK)
	body := resp.BodyString()
	if !contains(body, `"data"`) {
		t.Errorf("expected data in response: %s", body)
	}
}

func TestNewHandler_GETQuery(t *testing.T) {
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"ping": &graphql.Field{Type: graphql.String},
		},
	})
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, nil)
	s := testutil.NewServer(t, app)

	resp := s.GET("/graphql?query={ping}")
	resp.AssertStatus(http.StatusOK)
	if !contains(resp.BodyString(), `"data"`) {
		t.Errorf("expected data in GET response")
	}
}

func TestNewHandler_Mutation(t *testing.T) {
	var counter int
	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"increment": &graphql.Field{
				Type: graphql.Int,
				Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
					counter++
					return counter, nil
				},
			},
		},
	})
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"count": &graphql.Field{
				Type: graphql.Int,
				Resolve: func(_ graphql.ResolveParams) (interface{}, error) {
					return counter, nil
				},
			},
		},
	})
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, nil)
	s := testutil.NewServer(t, app)

	for i := 0; i < 2; i++ {
		s.POST("/graphql", map[string]string{"query": "mutation { increment }"})
	}
	resp := s.POST("/graphql", map[string]string{"query": "{ count }"})
	resp.AssertStatus(http.StatusOK)
	body := resp.BodyString()
	if !contains(body, `"count":2`) {
		t.Errorf("expected count=2, got: %s", body)
	}
}

func TestNewHandler_Variables(t *testing.T) {
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"greet": &graphql.Field{
				Type: graphql.String,
				Args: graphql.FieldConfigArgument{
					"name": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					name, _ := p.Args["name"].(string)
					if name == "" {
						return "stranger", nil
					}
					return "Hello, " + name, nil
				},
			},
		},
	})
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, nil)
	s := testutil.NewServer(t, app)

	body := map[string]interface{}{
		"query":     "query Greet($name: String!) { greet(name: $name) }",
		"variables": map[string]string{"name": "Alice"},
	}
	resp := s.POST("/graphql", body)
	resp.AssertStatus(http.StatusOK)
	respBody := resp.BodyString()
	if !contains(respBody, "Hello, Alice") {
		t.Errorf("expected greeting, got: %s", respBody)
	}
}

func TestNewHandler_InvalidJSON(t *testing.T) {
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"x": &graphql.Field{Type: graphql.String},
		},
	})
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, nil)
	s := testutil.NewServer(t, app)

	req, _ := http.NewRequest("POST", s.URL()+"/graphql", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	resp, _ := s.Client().Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

// ─── MapStruct ────────────────────────────────────────────────────────────────

func TestMapStruct_Basic(t *testing.T) {
	type User struct {
		ID    int64  `astra:"type:ID!"`
		Name  string `astra:"name:fullName"`
		Email string
	}
	obj := astragraphql.MapStruct(User{})
	fields := obj.Fields()
	if fields["fullName"] == nil {
		t.Errorf("expected field 'fullName', got: %v", fields)
	}
	if fields["Email"] == nil {
		t.Errorf("expected field 'Email', got: %v", fields)
	}
}

func TestMapStruct_SkipDash(t *testing.T) {
	type Person struct {
		Name string
		Pass string `astra:"-"`
	}
	obj := astragraphql.MapStruct(Person{})
	if obj.Fields()["Pass"] != nil {
		t.Error("'-' tag should skip the field")
	}
}

func TestMapStruct_ScalarTypes(t *testing.T) {
	type S struct {
		ID    int64    `astra:"type:ID!"`
		Int   int      `astra:"type:Int!"`
		Float float64  `astra:"type:Float!"`
		Bool  bool     `astra:"type:Boolean!"`
		Str   string   `astra:"type:String!"`
		List  []int    `astra:"type:[Int!]!"`
	}
	obj := astragraphql.MapStruct(S{})
	for _, name := range []string{"ID", "Int", "Float", "Bool", "Str", "List"} {
		if obj.Fields()[name] == nil {
			t.Errorf("expected field %q", name)
		}
	}
}

func hasArg(f *graphql.FieldDefinition, name string) bool {
	for _, a := range f.Args {
		if a.PrivateName == name {
			return true
		}
	}
	return false
}

func TestMapStruct_Args(t *testing.T) {
	type Query struct {
		_     struct{} `astra:"desc:Root query"`
		Users interface{} `astra:"type:[User!]!,arg:limit:Int!,arg:offset:Int"`
	}
	obj := astragraphql.MapStruct(Query{})
	f := obj.Fields()["Users"]
	if f == nil {
		t.Fatal("Users field missing")
	}
	if len(f.Args) == 0 {
		t.Fatal("args should not be empty")
	}
	if !hasArg(f, "limit") {
		t.Error("expected 'limit' argument")
	}
	if !hasArg(f, "offset") {
		t.Error("expected 'offset' argument")
	}
}

func TestMapStruct_Description(t *testing.T) {
	type Doc struct {
		Value int `astra:"desc:A numeric value"`
	}
	obj := astragraphql.MapStruct(Doc{})
	f := obj.Fields()["Value"]
	if f == nil {
		t.Fatal("Value field missing")
	}
	if f.Description != "A numeric value" {
		t.Errorf("expected description, got: %q", f.Description)
	}
}

func TestMapStruct_Deprecated(t *testing.T) {
	type Legacy struct {
		OldField string `astra:"deprecated:use newField instead"`
	}
	obj := astragraphql.MapStruct(Legacy{})
	f := obj.Fields()["OldField"]
	if f == nil {
		t.Fatal("OldField missing")
	}
	if f.DeprecationReason != "use newField instead" {
		t.Errorf("expected deprecation reason, got: %q", f.DeprecationReason)
	}
}

// ─── SchemaBuilder ────────────────────────────────────────────────────────────

func TestSchemaBuilder_QueryOnly(t *testing.T) {
	qType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"user": &graphql.Field{Type: graphql.String},
		},
	})
	schema, err := astragraphql.NewSchemaBuilder().WithQuery(qType).Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if schema.QueryType() == nil {
		t.Error("query type should be set")
	}
}

func TestSchemaBuilder_QueryAndMutation(t *testing.T) {
	qType := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"_": &graphql.Field{Type: graphql.Boolean},
	}})
	mType := graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: graphql.Fields{
		"_": &graphql.Field{Type: graphql.Boolean},
	}})
	schema, err := astragraphql.NewSchemaBuilder().WithQuery(qType).WithMutation(mType).Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if schema.MutationType() == nil {
		t.Error("mutation type should be set")
	}
}

func TestSchemaBuilder_Subscription(t *testing.T) {
	qType := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"_": &graphql.Field{Type: graphql.Boolean},
	}})
	sType := graphql.NewObject(graphql.ObjectConfig{Name: "Subscription", Fields: graphql.Fields{
		"_": &graphql.Field{Type: graphql.Boolean},
	}})
	schema, err := astragraphql.NewSchemaBuilder().WithQuery(qType).WithSubscription(sType).Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if schema.SubscriptionType() == nil {
		t.Error("subscription type should be set")
	}
}

func TestMustBuildSchema_PanicsWithoutQuery(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for schema with no query type")
		}
	}()
	_ = astragraphql.MustBuildSchema(astragraphql.NewSchemaBuilder())
}

// ─── SimpleResolver dispatch ──────────────────────────────────────────────────

func TestSimpleResolver_Dispatch(t *testing.T) {
	type User struct {
		ID   int64
		Name string
	}

	userType := astragraphql.MapStruct(User{})
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"user": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return &User{ID: 1, Name: "default"}, nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	resolver := astragraphql.SimpleResolver{
		"User.Name": func(p graphql.ResolveParams) (interface{}, error) {
			u := p.Source.(*User)
			return "resolved:" + u.Name, nil
		},
	}

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, resolver)
	s := testutil.NewServer(t, app)

	resp := s.POST("/graphql", map[string]string{"query": "{ user { Name } }"})
	resp.AssertStatus(http.StatusOK)
	body := resp.BodyString()
	if !contains(body, "resolved:default") {
		t.Errorf("SimpleResolver not dispatched; got: %s", body)
	}
}

// ─── MapStruct + MountSchema end-to-end ──────────────────────────────────────

func TestMapStruct_WithMountSchema_DefaultFieldResolver(t *testing.T) {
	type Product struct {
		ID    int64  `astra:"type:ID!"`
		Title string `astra:"name:title"`
		Price float64
	}

	productType := astragraphql.MapStruct(Product{})
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"product": &graphql.Field{
				Type: productType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return &Product{ID: 42, Title: "Widget", Price: 9.99}, nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, nil)
	s := testutil.NewServer(t, app)

	resp := s.POST("/graphql", map[string]string{"query": "{ product { title Price } }"})
	resp.AssertStatus(http.StatusOK)
	body := resp.BodyString()
	if !contains(body, "Widget") {
		t.Errorf("expected title 'Widget' in response; got: %s", body)
	}
	if !contains(body, "9.99") {
		t.Errorf("expected price 9.99 in response; got: %s", body)
	}
}

func TestMapStruct_WithMountSchema_SimpleResolverOverride(t *testing.T) {
	type Article struct {
		ID    int64
		Title string
	}

	articleType := astragraphql.MapStruct(Article{})
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"article": &graphql.Field{
				Type: articleType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return &Article{ID: 7, Title: "original"}, nil
				},
			},
		},
	})
	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	resolver := astragraphql.SimpleResolver{
		"Article.Title": func(p graphql.ResolveParams) (interface{}, error) {
			a := p.Source.(*Article)
			return "overridden:" + a.Title, nil
		},
	}

	app := testutil.NewTestApp()
	astragraphql.MountSchema(app, schema, resolver)
	s := testutil.NewServer(t, app)

	resp := s.POST("/graphql", map[string]string{"query": "{ article { Title } }"})
	resp.AssertStatus(http.StatusOK)
	body := resp.BodyString()
	if !contains(body, "overridden:original") {
		t.Errorf("SimpleResolver override not applied; got: %s", body)
	}
}