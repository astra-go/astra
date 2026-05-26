// Package astra is a modern, high-performance Go web framework that brings together
// the best features from gin, go-zero, beego, echo, and kratos.
//
// Astra (星辰) — Built for the stars.
//
// # Quick start (Gin/Echo-compatible)
//
//	app := astra.New()
//	app.GET("/hello/:name", func(c *astra.Ctx) error {
//	    return c.JSON(200, astra.Map{"hello": c.Param("name")})
//	})
//	app.Run(":8080")
//
// That's it. Middleware, route groups, DI, modules, and plugins are all
// available but entirely optional — introduce them only when you need them.
//
// See examples/hello for the minimal template, examples/quickstart for a
// real-service template, and docs/getting-started/quickstart.md for a
// progressive three-step guide.
package astra
