package middleware_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/astra-go/astra/testutil"
)

// ─── ResponseFormat ───────────────────────────────────────────────────────────

func TestResponseFormat_WrapsSuccessJSON(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormat())
	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"name": "alice"})
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)

	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got := env["code"]; got != float64(0) {
		t.Errorf("code: want 0, got %v", got)
	}
	if got := env["message"]; got != "ok" {
		t.Errorf("message: want \"ok\", got %v", got)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data: want object, got %T", env["data"])
	}
	if data["name"] != "alice" {
		t.Errorf("data.name: want \"alice\", got %v", data["name"])
	}
}

func TestResponseFormat_WrapsErrorJSON(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormat())
	app.GET("/", func(c *astra.Ctx) error {
		return astra.NewHTTPError(http.StatusNotFound, "not found")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusNotFound)

	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got := env["code"]; got != float64(http.StatusNotFound) {
		t.Errorf("code: want %d, got %v", http.StatusNotFound, got)
	}
	// message should be the original error body (a JSON object)
	if env["message"] == nil {
		t.Error("message: want non-nil error body")
	}
}

func TestResponseFormat_EmptySuccessBody(t *testing.T) {
	// A JSON handler that returns an empty object — data should be {} not null.
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormat())
	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{})
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)

	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if env["data"] == nil {
		t.Error("data: want non-nil for non-empty JSON body")
	}
}

func TestResponseFormat_NoContentPassthrough(t *testing.T) {
	// 204 No Content has no body and no Content-Type — should pass through unchanged.
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormat())
	app.GET("/", func(c *astra.Ctx) error {
		return c.NoContent(http.StatusNoContent)
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusNoContent)
	if body := resp.BodyString(); body != "" {
		t.Errorf("204 should have empty body, got %q", body)
	}
}

func TestResponseFormat_NonJSONPassthrough(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormat())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "hello")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK).AssertBodyContains("hello")

	// Body must NOT be wrapped in an envelope.
	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err == nil {
		if _, hasCode := env["code"]; hasCode {
			t.Error("plain-text response should not be wrapped in an envelope")
		}
	}
}

func TestResponseFormat_SkipperBypassesWrap(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormatWithConfig(middleware.ResponseFormatConfig{
		OnlyJSON: true,
		Skipper:  func(_ *astra.Ctx) bool { return true },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"x": 1})
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)

	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	// Skipper active — no envelope fields.
	if _, hasCode := env["code"]; hasCode {
		t.Error("skipped request should not have envelope code field")
	}
}

func TestResponseFormat_RequestIDIncluded(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.RequestID())
	app.Use(middleware.ResponseFormat())
	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{})
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)

	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	rid, _ := env["request_id"].(string)
	if rid == "" {
		t.Error("request_id: want non-empty string when RequestID middleware is active")
	}
}

func TestResponseFormat_CustomSuccessCode(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormatWithConfig(middleware.ResponseFormatConfig{
		SuccessCode:  200,
		OnlyJSON:     true,
		RequestIDKey: "",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{})
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got := env["code"]; got != float64(200) {
		t.Errorf("code: want 200, got %v", got)
	}
}

func TestResponseFormat_CustomErrorCodeMapper(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.ResponseFormatWithConfig(middleware.ResponseFormatConfig{
		OnlyJSON:        true,
		RequestIDKey:    "",
		ErrorCodeMapper: func(_ int) int { return -1 },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return astra.NewHTTPError(http.StatusBadRequest, "bad")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	var env map[string]any
	if err := json.Unmarshal([]byte(resp.BodyString()), &env); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got := env["code"]; got != float64(-1) {
		t.Errorf("code: want -1 from custom mapper, got %v", got)
	}
}
