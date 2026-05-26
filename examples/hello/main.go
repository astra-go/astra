// Minimal Astra example — comparable to a Gin/Echo hello-world.
// No DI, no modules, no plugins. Just routes and run.
package main

import (
	"net/http"

	"github.com/astra-go/astra"
)

func main() {
	app := astra.New()

	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"message": "hello astra"})
	})

	app.GET("/hello/:name", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"hello": c.Param("name")})
	})

	app.Run(":8080")
}
