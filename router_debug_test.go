package astra

import (
	"strings"
	"testing"
)

func TestRouter_Visualize_Empty(t *testing.T) {
	app := New()
	output := app.router.(*Router).Visualize()
	if output != "" {
		t.Errorf("expected empty output for router with no routes, got: %s", output)
	}
}

func TestRouter_Visualize_SingleStaticRoute(t *testing.T) {
	app := New()
	app.GET("/users", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()
	if !strings.Contains(output, "GET /") {
		t.Error("expected GET method in output")
	}
	if !strings.Contains(output, "/users") {
		t.Error("expected /users path in output")
	}
	if !strings.Contains(output, "handler") {
		t.Error("expected handler count in output")
	}
}

func TestRouter_Visualize_NestedStaticRoutes(t *testing.T) {
	app := New()
	app.GET("/users/list", func(c *Ctx) error { return nil })
	app.GET("/users/active", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()
	if !strings.Contains(output, "GET /") {
		t.Error("expected GET method in output")
	}
	if !strings.Contains(output, "/users") {
		t.Error("expected /users path in output")
	}
	if !strings.Contains(output, "/list") {
		t.Error("expected /list path in output")
	}
	if !strings.Contains(output, "/active") {
		t.Error("expected /active path in output")
	}
}

func TestRouter_Visualize_ParamNode(t *testing.T) {
	app := New()
	app.GET("/users/:id", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()
	if !strings.Contains(output, "[:id]") {
		t.Errorf("expected [:id] param node in output, got: %s", output)
	}
	if !strings.Contains(output, "handler") {
		t.Error("expected handler count in output")
	}
}

func TestRouter_Visualize_RegexNode(t *testing.T) {
	app := New()
	app.GET("/users/{id:[0-9]+}", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()
	if !strings.Contains(output, "[{id:[0-9]+}]") {
		t.Errorf("expected [{id:[0-9]+}] regex node in output, got: %s", output)
	}
}

func TestRouter_Visualize_CatchAllNode(t *testing.T) {
	app := New()
	app.GET("/static/*filepath", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()
	if !strings.Contains(output, "[*filepath]") {
		t.Errorf("expected [*filepath] catch-all node in output, got: %s", output)
	}
}

func TestRouter_Visualize_MixedNodeTypes(t *testing.T) {
	app := New()
	app.GET("/users/list", func(c *Ctx) error { return nil })
	app.GET("/users/:id", func(c *Ctx) error { return nil })
	app.GET("/users/:id/edit", func(c *Ctx) error { return nil })
	app.GET("/users/*rest", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()

	// Check for all node types
	if !strings.Contains(output, "/list") {
		t.Error("expected static node /list")
	}
	if !strings.Contains(output, "[:id]") {
		t.Error("expected param node [:id]")
	}
	if !strings.Contains(output, "/edit") {
		t.Error("expected nested /edit path")
	}
	if !strings.Contains(output, "[*rest]") {
		t.Error("expected catch-all node [*rest]")
	}

	// Check tree structure characters
	if !strings.Contains(output, "└─") {
		t.Error("expected tree structure with └─")
	}
	if !strings.Contains(output, "├─") {
		t.Error("expected tree structure with ├─")
	}
}

func TestRouter_Visualize_MultipleMethods(t *testing.T) {
	app := New()
	app.GET("/users", func(c *Ctx) error { return nil })
	app.POST("/users", func(c *Ctx) error { return nil })
	app.DELETE("/users/:id", func(c *Ctx) error { return nil })

	output := app.router.(*Router).Visualize()

	// Methods should be sorted alphabetically
	lines := strings.Split(output, "\n")
	methodsSeen := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "DELETE ") {
			methodsSeen = append(methodsSeen, "DELETE")
		} else if strings.HasPrefix(line, "GET ") {
			methodsSeen = append(methodsSeen, "GET")
		} else if strings.HasPrefix(line, "POST ") {
			methodsSeen = append(methodsSeen, "POST")
		}
	}

	if len(methodsSeen) != 3 {
		t.Errorf("expected 3 methods, got %d", len(methodsSeen))
	}

	// Verify alphabetical order
	if methodsSeen[0] != "DELETE" || methodsSeen[1] != "GET" || methodsSeen[2] != "POST" {
		t.Errorf("methods not in alphabetical order: %v", methodsSeen)
	}
}

func TestRouter_Visualize_MultipleHandlers(t *testing.T) {
	app := New()
	handler1 := func(c *Ctx) error { return nil }
	handler2 := func(c *Ctx) error { return nil }
	handler3 := func(c *Ctx) error { return nil }

	app.GET("/users", handler1, handler2, handler3)

	output := app.router.(*Router).Visualize()
	if !strings.Contains(output, "handlers)") {
		t.Errorf("expected multiple handlers in output, got: %s", output)
	}
}

func TestRouter_Visualize_ThreadSafety(t *testing.T) {
	app := New()
	app.GET("/users", func(c *Ctx) error { return nil })

	// Call Visualize concurrently to ensure thread safety
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			output := app.router.(*Router).Visualize()
			if !strings.Contains(output, "GET /") {
				t.Error("concurrent Visualize() call failed")
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRouter_Visualize_DeterministicOutput(t *testing.T) {
	app := New()
	app.GET("/users/active", func(c *Ctx) error { return nil })
	app.GET("/users/list", func(c *Ctx) error { return nil })
	app.GET("/users/archived", func(c *Ctx) error { return nil })

	// Call Visualize multiple times and ensure output is identical
	output1 := app.router.(*Router).Visualize()
	output2 := app.router.(*Router).Visualize()
	output3 := app.router.(*Router).Visualize()

	if output1 != output2 || output2 != output3 {
		t.Error("Visualize() output is not deterministic")
	}

	// Verify children are sorted
	lines := strings.Split(output1, "\n")
	paths := []string{}
	for _, line := range lines {
		if strings.Contains(line, "/active") {
			paths = append(paths, "active")
		} else if strings.Contains(line, "/archived") {
			paths = append(paths, "archived")
		} else if strings.Contains(line, "/list") {
			paths = append(paths, "list")
		}
	}

	if len(paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(paths))
	}

	// Verify alphabetical order
	if paths[0] != "active" || paths[1] != "archived" || paths[2] != "list" {
		t.Errorf("paths not in alphabetical order: %v", paths)
	}
}
