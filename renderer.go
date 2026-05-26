package astra

import "io"

// Renderer is the interface that template engines must implement.
// Register an engine via astra.WithRenderer; call it via c.Render.
//
// The render sub-package provides a production-ready HTMLEngine:
//
//	import "github.com/astra-go/astra/render"
//
//	engine := render.Must(render.Config{
//	    Root:   "templates",
//	    Layout: "layouts/base.html",
//	})
//	app := astra.New(astra.WithRenderer(engine))
type Renderer interface {
	Render(w io.Writer, name string, data any) error
}
