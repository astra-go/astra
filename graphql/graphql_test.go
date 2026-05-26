package graphql_test

import (
	"net/http"
	"testing"

	"github.com/astra-go/astra/graphql"
	"github.com/astra-go/astra/testutil"
)

// echoHandler returns a minimal http.Handler that echoes "ok" for any request.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":"ok"}`))
	})
}

// ─── Mount — default paths ────────────────────────────────────────────────────

func TestMount_DefaultPath_GET(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler())
	s := testutil.NewServer(t, app)

	s.GET("/graphql").AssertStatus(http.StatusOK)
}

func TestMount_DefaultPath_POST(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler())
	s := testutil.NewServer(t, app)

	s.POST("/graphql", nil).AssertStatus(http.StatusOK)
}

func TestMount_DefaultPlayground(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler())
	s := testutil.NewServer(t, app)

	resp := s.GET("/playground")
	resp.AssertStatus(http.StatusOK)
	body := resp.BodyString()
	if body == "" {
		t.Error("playground response body is empty")
	}
	// Must contain basic HTML structure.
	if !containsAny(body, "<html", "GraphQL") {
		t.Errorf("playground HTML appears malformed: %.200s", body)
	}
}

// ─── Mount — custom paths ─────────────────────────────────────────────────────

func TestMount_CustomPath(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler(), graphql.Options{
		Path: "/api/graphql",
	})
	s := testutil.NewServer(t, app)

	s.GET("/api/graphql").AssertStatus(http.StatusOK)
	s.POST("/api/graphql", nil).AssertStatus(http.StatusOK)
}

func TestMount_CustomPlaygroundPath(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler(), graphql.Options{
		Path:           "/gql",
		PlaygroundPath: "/gql/ui",
	})
	s := testutil.NewServer(t, app)

	s.GET("/gql/ui").AssertStatus(http.StatusOK)
}

func TestMount_PlaygroundContainsEndpoint(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler(), graphql.Options{
		Path:           "/my-api",
		PlaygroundPath: "/play",
	})
	s := testutil.NewServer(t, app)

	body := s.GET("/play").BodyString()
	// The playground HTML must reference the graphql endpoint.
	if !containsAny(body, "/my-api") {
		t.Errorf("playground HTML should contain endpoint /my-api; got: %.200s", body)
	}
}

func TestMount_CustomPlaygroundTitle(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler(), graphql.Options{
		PlaygroundTitle: "My Custom API",
		PlaygroundPath:  "/play",
	})
	s := testutil.NewServer(t, app)

	body := s.GET("/play").BodyString()
	if !containsAny(body, "My Custom API") {
		t.Errorf("playground title not found in HTML: %.200s", body)
	}
}

// ─── Mount — disabled playground ─────────────────────────────────────────────

func TestMount_DisabledPlayground_NoPlaygroundRoute(t *testing.T) {
	app := testutil.NewTestApp()
	graphql.Mount(app, echoHandler(), graphql.Options{
		Path:           "/graphql",
		PlaygroundPath: "", // explicitly disabled
	})
	s := testutil.NewServer(t, app)

	// /playground must not be registered.
	s.GET("/playground").AssertStatus(http.StatusNotFound)
}

// ─── Handler forwarding ───────────────────────────────────────────────────────

func TestMount_HandlerReceivesRequest(t *testing.T) {
	received := false
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		received = true
	})

	app := testutil.NewTestApp()
	graphql.Mount(app, h)
	s := testutil.NewServer(t, app)

	s.POST("/graphql", map[string]string{"query": "{ hello }"})

	if !received {
		t.Error("expected the GraphQL handler to be called")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
