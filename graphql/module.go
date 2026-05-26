package graphql

import (
	"net/http"

	"github.com/astra-go/astra"
)

// NewModule returns an astra.Module that mounts a GraphQL handler (and
// optional Playground) when installed on an *App.
//
// h is typically a handler created by your schema library, e.g.:
//
//	import "github.com/99designs/gqlgen/graphql/handler"
//	srv := handler.NewDefaultServer(generated.NewExecutableSchema(cfg))
//
//	app.Register(graphql.NewModule(srv, graphql.Options{
//	    Path:           "/graphql",
//	    PlaygroundPath: "/playground",
//	}))
func NewModule(h http.Handler, opts ...Options) astra.Module {
	return astra.NewModuleFunc("graphql", func(app *astra.App) error {
		Mount(app, h, opts...)
		return nil
	})
}
