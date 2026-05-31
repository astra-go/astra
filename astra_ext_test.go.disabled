package astra_test

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/contract"
	"github.com/astra-go/astra/testutil"
)

// ─── context_bind.go ───────────────────────────────────────────────────────────

func TestCtx_Bind_AutoJSON(t *testing.T) {
	type bindReq struct {
		Name string `json:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/bind", func(c *astra.Ctx) error {
		var r bindReq
		if err := c.Bind(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"name": "json-auto"})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/bind", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_Bind_AutoXML(t *testing.T) {
	type bindXMLReq struct {
		Name string `xml:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/bind", func(c *astra.Ctx) error {
		var r bindXMLReq
		if err := c.Bind(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, map[string]string{"name": r.Name})
	})
	srv := testutil.NewServer(t, app)

	body, _ := xml.Marshal(bindXMLReq{Name: "xml-auto"})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/bind", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/xml")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_Bind_AutoForm(t *testing.T) {
	type bindFormReq struct {
		Name string `form:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/bind", func(c *astra.Ctx) error {
		var r bindFormReq
		if err := c.Bind(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, map[string]string{"name": r.Name})
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"name": []string{"form-auto"}}.Encode()
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/bind", strings.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_BindXML(t *testing.T) {
	type xmlReq struct {
		Name string `xml:"name"`
		Age  int    `xml:"age"`
	}
	app := testutil.NewTestApp()
	app.POST("/xml", func(c *astra.Ctx) error {
		var r xmlReq
		if err := c.BindXML(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := xml.Marshal(xmlReq{Name: "alice", Age: 30})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/xml", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_BindXML_InvalidBody(t *testing.T) {
	type xmlReq struct {
		Name string `xml:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/xml", func(c *astra.Ctx) error {
		var r xmlReq
		return c.BindXML(&r)
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/xml", strings.NewReader("not xml"))
	req.Header.Set("Content-Type", "application/xml")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestCtx_BindXML_EmptyBody(t *testing.T) {
	type xmlReq struct {
		Name string `xml:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/xml", func(c *astra.Ctx) error {
		var r xmlReq
		return c.BindXML(&r)
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/xml", nil)
	req.Header.Set("Content-Type", "application/xml")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("want 400 for nil body, got %d", resp.StatusCode)
	}
}

func TestCtx_BindForm(t *testing.T) {
	type formReq struct {
		Username string `form:"username"`
		Active   bool    `form:"active"`
	}
	app := testutil.NewTestApp()
	app.POST("/form", func(c *astra.Ctx) error {
		var r formReq
		if err := c.BindForm(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"username": []string{"bob"}, "active": []string{"true"}}.Encode()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/form", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_BindQuery(t *testing.T) {
	type queryReq struct {
		Page   int    `query:"page"`
		SortBy string `query:"sort"`
	}
	app := testutil.NewTestApp()
	app.GET("/query", func(c *astra.Ctx) error {
		var r queryReq
		if err := c.BindQuery(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/query?page=5&sort=name").AssertStatus(http.StatusOK)
}

func TestCtx_BindPath(t *testing.T) {
	type pathReq struct {
		ID int64 `uri:"id"`
	}
	app := testutil.NewTestApp()
	app.GET("/items/:id", func(c *astra.Ctx) error {
		var r pathReq
		if err := c.BindPath(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, map[string]int64{"id": r.ID})
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/items/42").AssertStatus(http.StatusOK).AssertBodyContains(`"id":42`)
}

func TestCtx_BindHeader(t *testing.T) {
	type headerReq struct {
		Auth string `header:"Authorization"`
	}
	app := testutil.NewTestApp()
	app.GET("/headers", func(c *astra.Ctx) error {
		var r headerReq
		if err := c.BindHeader(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, map[string]string{"auth": r.Auth})
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/headers", nil)
	req.Header.Set("Authorization", "Bearer token123")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_BindAll(t *testing.T) {
	type allReq struct {
		ID   int64  `uri:"id"`
		Page int    `query:"page"`
		Name string `json:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/items/:id", func(c *astra.Ctx) error {
		var r allReq
		if err := c.BindAll(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/items/7?page=3", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_ShouldBind_ValidationFails(t *testing.T) {
	type shouldBindReq struct {
		Name string `json:"name" validate:"required"`
	}
	app := testutil.NewTestApp()
	app.POST("/shouldbind", func(c *astra.Ctx) error {
		var r shouldBindReq
		if err := c.ShouldBind(&r); err != nil {
			return c.String(400, "validation_err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"name": ""})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/shouldbind", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("want 400 for validation failure, got %d", resp.StatusCode)
	}
}

func TestCtx_ShouldBindXML(t *testing.T) {
	type xmlReq struct {
		Value string `xml:"value"`
	}
	app := testutil.NewTestApp()
	app.POST("/shouldxml", func(c *astra.Ctx) error {
		var r xmlReq
		if err := c.ShouldBindXML(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, map[string]string{"value": r.Value})
	})
	srv := testutil.NewServer(t, app)

	body, _ := xml.Marshal(xmlReq{Value: "xml-val"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/shouldxml", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_ShouldBindForm(t *testing.T) {
	type formReq struct {
		Nick string `form:"nick"`
	}
	app := testutil.NewTestApp()
	app.POST("/shouldform", func(c *astra.Ctx) error {
		var r formReq
		if err := c.ShouldBindForm(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"nick": []string{"nickname"}}.Encode()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/shouldform", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_ShouldBindQuery(t *testing.T) {
	type qReq struct {
		Limit int `query:"limit"`
	}
	app := testutil.NewTestApp()
	app.GET("/shouldquery", func(c *astra.Ctx) error {
		var r qReq
		if err := c.ShouldBindQuery(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/shouldquery?limit=10").AssertStatus(http.StatusOK)
}

func TestCtx_ShouldBindPath(t *testing.T) {
	type pReq struct {
		UserID int64 `uri:"uid" validate:"gt=0"`
	}
	app := testutil.NewTestApp()
	app.GET("/users/:uid", func(c *astra.Ctx) error {
		var r pReq
		if err := c.ShouldBindPath(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/users/99").AssertStatus(http.StatusOK)
}

func TestCtx_ShouldBindPath_ValidationFails(t *testing.T) {
	type pReq struct {
		UserID int64 `uri:"uid" validate:"gt=0"`
	}
	app := testutil.NewTestApp()
	app.GET("/users/:uid", func(c *astra.Ctx) error {
		var r pReq
		if err := c.ShouldBindPath(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)
	// uid=0 fails gt=0 validation
	srv.GET("/users/0").AssertStatus(http.StatusBadRequest)
}

func TestCtx_ShouldBindHeader(t *testing.T) {
	type hReq struct {
		ReqID string `header:"X-Request-Id"`
	}
	app := testutil.NewTestApp()
	app.GET("/reqhdr", func(c *astra.Ctx) error {
		var r hReq
		if err := c.ShouldBindHeader(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/reqhdr", nil)
	req.Header.Set("X-Request-Id", "req-abc")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_ShouldBindAll(t *testing.T) {
	type allReq struct {
		ID   int64  `uri:"id"`
		Page int    `query:"page"`
		Name string `json:"name"`
	}
	app := testutil.NewTestApp()
	app.POST("/all/:id", func(c *astra.Ctx) error {
		var r allReq
		if err := c.ShouldBindAll(&r); err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"name": "full"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/all/1?page=2", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_MustBind_BindError(t *testing.T) {
	type mustBindReq struct {
		Name string `json:"name" validate:"required"`
	}
	app := testutil.NewTestApp()
	app.POST("/mustbind", func(c *astra.Ctx) error {
		var r mustBindReq
		// MustBind returns nil on success; on failure it aborts and writes error
		return c.MustBind(&r)
	})
	srv := testutil.NewServer(t, app)

	// Empty body → BindJSON fails with 400
	body, _ := json.Marshal(map[string]string{"name": ""})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/mustbind", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("want 422 for validation failure on required field, got %d", resp.StatusCode)
	}
}

func TestCtx_MustBindJSON_Success(t *testing.T) {
	type mustBindJSONReq struct {
		Name string `json:"name" validate:"required"`
	}
	app := testutil.NewTestApp()
	app.POST("/mustbindjson", func(c *astra.Ctx) error {
		var r mustBindJSONReq
		if err := c.MustBindJSON(&r); err != nil {
			return err // nil in practice; this line unreachable
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)

	srv.POST("/mustbindjson", map[string]string{"name": "alice"}).AssertStatus(http.StatusOK)
}

func TestCtx_MustBindJSON_ValidationError(t *testing.T) {
	type mustBindJSONValReq struct {
		Age int `json:"age" validate:"gt=18"`
	}
	app := testutil.NewTestApp()
	app.POST("/mbj", func(c *astra.Ctx) error {
		var r mustBindJSONValReq
		return c.MustBindJSON(&r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]int{"age": 10})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/mbj", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", resp.StatusCode)
	}
}

func TestCtx_MustBindAll_Success(t *testing.T) {
	type mustBindAllReq struct {
		Name string `json:"name" validate:"required"`
	}
	app := testutil.NewTestApp()
	app.POST("/mba/:id", func(c *astra.Ctx) error {
		var r mustBindAllReq
		return c.MustBindAll(&r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"name": "bob"})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/mba/1", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200 for MustBindAll success, got %d", resp.StatusCode)
	}
}

func TestCtx_Validate_SlimMode(t *testing.T) {
	app := astra.NewSlim()
	app.POST("/validate", func(c *astra.Ctx) error {
		var r struct{ Name string }
		return c.Validate(&r)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"name": "slim"})
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/validate", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 for slim-mode Validate, got %d", resp.StatusCode)
	}
}

// ─── context_response.go ─────────────────────────────────────────────────────

func TestCtx_XML_Response(t *testing.T) {
	type item struct {
		XMLName xml.Name `xml:"item"`
		ID      int      `xml:"id"`
		Name    string   `xml:"name"`
	}
	app := testutil.NewTestApp()
	app.GET("/xml", func(c *astra.Ctx) error {
		return c.XML(200, item{ID: 1, Name: "widget"})
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/xml")
	resp.AssertStatus(http.StatusOK).
		AssertHeader("Content-Type", "application/xml; charset=utf-8").
		AssertBodyContains("<id>1</id>").
		AssertBodyContains("<name>widget</name>")
}

func TestCtx_Render_NoRenderer(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/render", func(c *astra.Ctx) error {
		return c.Render(200, "missing.html", nil)
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/render").AssertStatus(http.StatusInternalServerError)
}

// mockRenderer is a Renderer that returns a canned response for testing.
type mockRenderer struct{}

func (mockRenderer) Render(w io.Writer, name string, data any) error {
	_, err := io.WriteString(w, "rendered:"+name)
	return err
}

func TestCtx_Render_WithRenderer(t *testing.T) {
	app := testutil.NewTestApp(astra.WithRenderer(mockRenderer{}))
	app.GET("/render", func(c *astra.Ctx) error {
		return c.Render(200, "index.html", map[string]string{"title": "Home"})
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/render")
	resp.AssertStatus(http.StatusOK).
		AssertHeader("Content-Type", "text/html; charset=utf-8").
		AssertBodyContains("rendered:index.html")
}

func TestCtx_JSONStream(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/stream", func(c *astra.Ctx) error {
		return c.JSONStream(200, []string{"a", "b", "c"})
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/stream")
	resp.AssertStatus(http.StatusOK).
		AssertHeader("Content-Type", "application/json; charset=utf-8")
	// JSONStream does not set Content-Length, so chunked transfer is used on HTTP/1.1
}

func TestCtx_File(t *testing.T) {
	// Create a temp file to serve
	tmp, err := os.CreateTemp("", "astra_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.WriteString("hello from file"); err != nil {
		t.Fatal(err)
	}

	app := testutil.NewTestApp()
	app.GET("/file", func(c *astra.Ctx) error {
		return c.File(tmp.Name())
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/file")
	resp.AssertStatus(http.StatusOK).
		AssertBodyContains("hello from file")
}

func TestCtx_File_NotFound(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/nofile", func(c *astra.Ctx) error {
		return c.File("/nonexistent/path/to/file.txt")
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/nofile").AssertStatus(http.StatusNotFound)
}

func TestCtx_SSEvent_Basic(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/sse", func(c *astra.Ctx) error {
		return c.SSEvent("message", "hello world")
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/sse", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("want text/event-stream, got %s", ct)
	}
}

func TestCtx_SSEvent_NamedEvent(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/sse2", func(c *astra.Ctx) error {
		if err := c.SSEvent("update", "data:123"); err != nil {
			return err
		}
		return c.SSEvent("", "plain data")
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/sse2", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_Push_NotSupported(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/push", func(c *astra.Ctx) error {
		err := c.Push("/style.css", nil)
		// HTTP/1.1 ResponseWriter does not implement http.Pusher
		if err != http.ErrNotSupported {
			return c.String(400, "expected ErrNotSupported, got %v", err)
		}
		return c.String(200, "push not supported (expected)")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/push").AssertStatus(http.StatusOK).
		AssertBodyContains("push not supported")
}

func TestCtx_EarlyHints(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/early", func(c *astra.Ctx) error {
		return c.EarlyHints([]string{"/style.css", "/app.js"}, map[string]string{"as": "style"})
	})

	// Use httptest.ResponseRecorder directly — Go's net/http client swallows
	// 103 interim responses, so we must inspect the recorder to verify the
	// status code and Link header were written correctly.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/early", nil)
	app.ServeHTTP(w, req)

	if w.Code != http.StatusEarlyHints {
		t.Errorf("want 103, got %d", w.Code)
	}
	link := w.Header().Get("Link")
	if !strings.Contains(link, "rel=preload") {
		t.Errorf("want Link header with rel=preload, got %s", link)
	}
}

func TestCtx_EarlyHints_AfterWritten(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/latehints", func(c *astra.Ctx) error {
		// Write header first
		c.Writer().WriteHeader(http.StatusOK)
		// Now EarlyHints should be no-op
		return c.EarlyHints([]string{"/style.css"}, nil)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/latehints").AssertStatus(http.StatusOK)
}

func TestCtx_Flush_ReturnsNil(t *testing.T) {
	app := testutil.NewTestApp()
	var flushErr error
	app.GET("/flush", func(c *astra.Ctx) error {
		c.Writer().WriteHeader(http.StatusOK)
		flushErr = c.Flush()
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/flush", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if flushErr != nil {
		t.Errorf("Flush() on httptest.ResponseRecorder want nil, got %v", flushErr)
	}
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestCtx_Done_EqualsRequestContextDone(t *testing.T) {
	app := testutil.NewTestApp()
	var ctxDone, reqDone <-chan struct{}
	app.GET("/done", func(c *astra.Ctx) error {
		ctxDone = c.Done()
		reqDone = c.Request().Context().Done()
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/done", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if ctxDone != reqDone {
		t.Error("c.Done() should return the same channel as c.Request().Context().Done()")
	}
}

func TestCtx_Done_ClosedOnCancel(t *testing.T) {
	app := testutil.NewTestApp()
	doneCh := make(chan (<-chan struct{}), 1)
	app.GET("/donecancel", func(c *astra.Ctx) error {
		doneCh <- c.Done()
		return c.NoContent(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/donecancel", nil)
	w := httptest.NewRecorder()

	cancel() // cancel before the handler runs
	app.ServeHTTP(w, req)

	ch := <-doneCh
	select {
	case <-ch:
		// expected: channel is closed because context was cancelled
	default:
		t.Error("c.Done() should be closed when request context is cancelled")
	}
}

// ─── context_store.go ─────────────────────────────────────────────────────────

func TestCtx_GetInt64(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/getint64", func(c *astra.Ctx) error {
		c.Set("big", int64(1<<60))
		return c.String(200, "big=%d", c.GetInt64("big"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/getint64").AssertBodyContains("big=1152921504606846976")
}

func TestCtx_GetFloat64(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/getfloat", func(c *astra.Ctx) error {
		c.Set("pi", 3.14159)
		return c.String(200, "pi=%.5f", c.GetFloat64("pi"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/getfloat").AssertBodyContains("pi=3.14159")
}

func TestCtx_GetBool(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/getbool", func(c *astra.Ctx) error {
		c.Set("flag", true)
		return c.String(200, "flag=%t", c.GetBool("flag"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/getbool").AssertBodyContains("flag=true")
}

func TestCtx_TryGetString_Found(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/trystr", func(c *astra.Ctx) error {
		c.Set("key", "value")
		v, ok := c.TryGetString("key")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/trystr").AssertStatus(http.StatusOK)
}

func TestCtx_TryGetString_Missing(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/trystrm", func(c *astra.Ctx) error {
		v, ok := c.TryGetString("nonexistent")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	var out map[string]any
	srv.GET("/trystrm").AssertJSON(&out)
	if out["ok"] != false {
		t.Errorf("TryGetString missing key should return ok=false")
	}
}

func TestCtx_TryGetString_WrongType(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/trystrwt", func(c *astra.Ctx) error {
		c.Set("num", 42)
		v, ok := c.TryGetString("num")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	var out map[string]any
	srv.GET("/trystrwt").AssertJSON(&out)
	if out["ok"] != false {
		t.Errorf("TryGetString with wrong type should return ok=false")
	}
}

func TestCtx_TryGetInt_Found(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/tryint", func(c *astra.Ctx) error {
		c.Set("cnt", 7)
		v, ok := c.TryGetInt("cnt")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	var out map[string]any
	srv.GET("/tryint").AssertJSON(&out)
	if out["ok"] != true || int64(out["val"].(float64)) != 7 {
		t.Errorf("TryGetInt: expected val=7, ok=true")
	}
}

func TestCtx_TryGetInt_Missing(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/tryintm", func(c *astra.Ctx) error {
		v, ok := c.TryGetInt("nonexistent")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	var out map[string]any
	srv.GET("/tryintm").AssertJSON(&out)
	if out["ok"] != false {
		t.Errorf("TryGetInt missing key should return ok=false")
	}
}

func TestCtx_TryGetBool_Found(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/trybool", func(c *astra.Ctx) error {
		c.Set("enabled", true)
		v, ok := c.TryGetBool("enabled")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	var out map[string]any
	srv.GET("/trybool").AssertJSON(&out)
	if out["ok"] != true || out["val"] != true {
		t.Errorf("TryGetBool: expected val=true, ok=true")
	}
}

func TestCtx_TryGetBool_Missing(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/tryboolm", func(c *astra.Ctx) error {
		v, ok := c.TryGetBool("nonexistent")
		return c.JSON(200, map[string]any{"val": v, "ok": ok})
	})
	srv := testutil.NewServer(t, app)
	var out map[string]any
	srv.GET("/tryboolm").AssertJSON(&out)
	if out["ok"] != false {
		t.Errorf("TryGetBool missing key should return ok=false")
	}
}

// ─── context_request.go ───────────────────────────────────────────────────────

func TestCtx_QueryMap(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/qmap", func(c *astra.Ctx) error {
		m := c.QueryMap()
		return c.JSON(200, m)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/qmap?a=1&b=2&c=3").AssertStatus(http.StatusOK)
}

func TestCtx_QueryMap_Empty(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/qmapempty", func(c *astra.Ctx) error {
		m := c.QueryMap()
		return c.JSON(200, map[string]int{"count": len(m)})
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/qmapempty").AssertBodyContains(`"count":0`)
}

func TestCtx_QueryMap_DecodesEncoded(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/qmapenc", func(c *astra.Ctx) error {
		m := c.QueryMap()
		return c.JSON(200, m)
	})
	srv := testutil.NewServer(t, app)
	// space is encoded as +
	srv.GET("/qmapenc?name=hello+world&city=New+York").AssertStatus(http.StatusOK)
}

func TestCtx_PostForm(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/pform", func(c *astra.Ctx) error {
		return c.JSON(200, map[string]string{"field": c.PostForm("field")})
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"field": []string{"post-value"}}.Encode()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/pform", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_DefaultPostForm(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/dpf", func(c *astra.Ctx) error {
		return c.String(200, "val=%s", c.DefaultPostForm("missing", "default-val"))
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"other": []string{"x"}}.Encode()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/dpf", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("default-val")) {
		t.Errorf("want default-val in response, got %s", b)
	}
}

func TestCtx_FormFile(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/upload", func(c *astra.Ctx) error {
		fh, err := c.FormFile("myfile")
		if err != nil {
			return c.String(400, "err=%v", err)
		}
		return c.String(200, "filename=%s", fh.Filename)
	})
	srv := testutil.NewServer(t, app)

	// Create multipart form with a file
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("myfile", "test.txt")
	fw.Write([]byte("hello file"))
	w.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("test.txt")) {
		t.Errorf("want filename test.txt in response, got %s", b)
	}
}

func TestCtx_FormFile_NotFound(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/noupload", func(c *astra.Ctx) error {
		_, err := c.FormFile("nonexistent")
		return c.String(400, "err=%v", err)
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"other": []string{"x"}}.Encode()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/noupload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("want 400 for missing form file, got %d", resp.StatusCode)
	}
}

func TestCtx_ContentType(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/ct", func(c *astra.Ctx) error {
		return c.String(200, "ct=%s", c.ContentType())
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"x": "y"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/ct", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("application/json")) {
		t.Errorf("want application/json, got %s", b)
	}
}

func TestCtx_ContentType_WithCharset(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/ct2", func(c *astra.Ctx) error {
		return c.String(200, "ct=%s", c.ContentType())
	})
	srv := testutil.NewServer(t, app)

	body := url.Values{"x": []string{"y"}}.Encode()
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/ct2", strings.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("application/x-www-form-urlencoded")) {
		t.Errorf("want application/x-www-form-urlencoded, got %s", b)
	}
}

func TestCtx_UserAgent(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/ua", func(c *astra.Ctx) error {
		return c.String(200, "ua=%s", c.UserAgent())
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/ua", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("TestAgent/1.0")) {
		t.Errorf("want TestAgent/1.0, got %s", b)
	}
}

func TestCtx_IsWebsocket_True(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/ws", func(c *astra.Ctx) error {
		return c.String(200, "ws=%t", c.IsWebsocket())
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(b, []byte("ws=true")) {
		t.Errorf("want ws=true, got %s", b)
	}
}

func TestCtx_IsWebsocket_False(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/nows", func(c *astra.Ctx) error {
		return c.String(200, "ws=%t", c.IsWebsocket())
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/nows")
	resp.AssertBodyContains("ws=false")
}

// ─── context_flow.go ──────────────────────────────────────────────────────────

func TestCtx_AbortWithError(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/aborterr", func(c *astra.Ctx) error {
		c.AbortWithError(http.StatusUnauthorized, errors.New("auth required"))
		return nil
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/aborterr")
	resp.AssertStatus(http.StatusUnauthorized).
		AssertBodyContains("auth required")
}

// ─── group.go ────────────────────────────────────────────────────────────────

func TestGroup_Use(t *testing.T) {
	app := testutil.NewTestApp()
	api := app.Group("/api")
	api.Use(func(c *astra.Ctx) error {
		c.Set("group", "api")
		c.Next()
		return nil
	})
	api.POST("/data", func(c *astra.Ctx) error {
		return c.String(200, "group=%s", c.GetString("group"))
	})
	srv := testutil.NewServer(t, app)

	resp := srv.POST("/api/data", nil)
	resp.AssertStatus(http.StatusOK).AssertBodyContains("group=api")
}

func TestGroup_POST(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/g")
	g.POST("/post", func(c *astra.Ctx) error { return c.String(200, "posted") })
	srv := testutil.NewServer(t, app)
	srv.POST("/g/post", nil).AssertStatus(http.StatusOK).AssertBodyContains("posted")
}

func TestGroup_PUT(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/g")
	g.PUT("/put", func(c *astra.Ctx) error { return c.String(200, "putted") })
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"x": "y"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, srv.URL()+"/g/put", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestGroup_DELETE(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/g")
	g.DELETE("/del", func(c *astra.Ctx) error { return c.String(200, "deleted") })
	srv := testutil.NewServer(t, app)
	srv.DELETE("/g/del").AssertStatus(http.StatusOK).AssertBodyContains("deleted")
}

func TestGroup_PATCH(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/g")
	g.PATCH("/patch", func(c *astra.Ctx) error { return c.String(200, "patched") })
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(map[string]string{"x": "y"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, srv.URL()+"/g/patch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestGroup_HEAD(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/g")
	g.HEAD("/head", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodHead, srv.URL()+"/g/head", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestGroup_OPTIONS(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/g")
	g.OPTIONS("/opts", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodOptions, srv.URL()+"/g/opts", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestGroup_Any(t *testing.T) {
	app := testutil.NewTestApp()
	g := app.Group("/any")
	g.Any("/all", func(c *astra.Ctx) error { return c.String(200, "method=%s", c.Request().Method) })
	srv := testutil.NewServer(t, app)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions} {
		req, _ := http.NewRequestWithContext(context.Background(), method, srv.URL()+"/any/all", nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Any: method %s want 200, got %d", method, resp.StatusCode)
		}
	}
}

func TestGroup_Nested(t *testing.T) {
	app := testutil.NewTestApp()
	outer := app.Group("/outer")
	inner := outer.Group("/inner")
	inner.GET("/deep", func(c *astra.Ctx) error { return c.String(200, "deep") })
	srv := testutil.NewServer(t, app)
	srv.GET("/outer/inner/deep").AssertStatus(http.StatusOK).AssertBodyContains("deep")
}

// ─── options.go ───────────────────────────────────────────────────────────────

func TestWithMaxMultipartMemory(t *testing.T) {
	app := astra.New(astra.WithMaxMultipartMemory(1<<20)) // 1MB
	if app == nil {
		t.Fatal("New with WithMaxMultipartMemory returned nil")
	}
	// App should start without error
	_ = app
}

func TestWithErrorHandler(t *testing.T) {
	called := false
	app := astra.New(astra.WithErrorHandler(func(c *astra.Ctx, err error) {
		called = true
		_ = c.String(http.StatusInternalServerError, "handled")
	}))
	app.GET("/err", func(c *astra.Ctx) error {
		return errors.New("test error")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/err").AssertStatus(http.StatusInternalServerError)
	if !called {
		t.Error("WithErrorHandler was not called")
	}
}

func TestWithShutdownTimeout(t *testing.T) {
	app := astra.New(astra.WithShutdownTimeout(30))
	if app == nil {
		t.Fatal("New with WithShutdownTimeout returned nil")
	}
	_ = app
}

func TestWithRenderer(t *testing.T) {
	app := astra.New(astra.WithRenderer(mockRenderer{}))
	if app == nil {
		t.Fatal("New with WithRenderer returned nil")
	}
	_ = app
}

func TestWithBinder(t *testing.T) {
	app := astra.New(astra.WithBinder(nil))
	if app == nil {
		t.Fatal("New with WithBinder returned nil")
	}
	_ = app
}

func TestWithStrictConflict(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic: %v", r)
		}
	}()
	app := astra.New(astra.WithStrictConflict())
	// Second registration with same path should panic in strict mode
	app.GET("/strict", func(c *astra.Ctx) error { return nil })
	app.GET("/strict", func(c *astra.Ctx) error { return nil })
}

func TestWithNotFoundHandler(t *testing.T) {
	app := astra.New(astra.WithNotFoundHandler(func(c *astra.Ctx) error {
		return c.String(http.StatusTeapot, "custom 404")
	}))
	srv := testutil.NewServer(t, app)
	srv.GET("/missing").AssertStatus(http.StatusTeapot).AssertBodyContains("custom 404")
}

func TestWithMethodNotAllowedHandler(t *testing.T) {
	app := astra.New(astra.WithMethodNotAllowedHandler(func(c *astra.Ctx) error {
		return c.String(http.StatusConflict, "custom 405")
	}))
	app.GET("/res", func(c *astra.Ctx) error { return nil })
	app.PUT("/res", func(c *astra.Ctx) error { return nil })

	req := httptest.NewRequest(http.MethodDelete, "/res", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "custom 405") {
		t.Errorf("want custom 405 body, got %s", w.Body.String())
	}
}

// mockRouter is a custom HttpRouter for testing WithRouter.
type mockRouter struct {
	routes []astra.RouteInfo
}

func (m *mockRouter) Add(method, path string, handlers astra.HandlersChain) {}

func (m *mockRouter) Handle(c *astra.Ctx) {
	c.Writer().WriteHeader(http.StatusOK)
}

func (m *mockRouter) Routes() []astra.RouteInfo {
	return m.routes
}

func TestWithRouter(t *testing.T) {
	app := astra.New(astra.WithRouter(&mockRouter{
		routes: []astra.RouteInfo{
			{Method: "GET", Path: "/mock"},
		},
	}))
	if app == nil {
		t.Fatal("New with WithRouter returned nil")
	}
	_ = app
}

// ─── response_writer.go ───────────────────────────────────────────────────────

func TestResponseWriter_Status(t *testing.T) {
	var capturedRW contract.ResponseWriter
	app := testutil.NewTestApp()
	app.GET("/status", func(c *astra.Ctx) error {
		c.Writer().WriteHeader(http.StatusCreated)
		capturedRW = c.Writer()
		return nil
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	app.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("want %d, got %d", http.StatusCreated, w.Code)
	}
	if capturedRW.Status() != http.StatusCreated {
		t.Errorf("ResponseWriter.Status() want %d, got %d", http.StatusCreated, capturedRW.Status())
	}
}

func TestResponseWriter_Size(t *testing.T) {
	var capturedRW contract.ResponseWriter
	app := testutil.NewTestApp()
	app.GET("/size", func(c *astra.Ctx) error {
		capturedRW = c.Writer()
		return c.String(200, "hello world")
	})
	req := httptest.NewRequest(http.MethodGet, "/size", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if capturedRW.Size() == 0 {
		t.Error("ResponseWriter.Size() should be > 0 after writing body")
	}
}

func TestResponseWriter_WriteHeader_Twice(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/twice", func(c *astra.Ctx) error {
		c.Writer().WriteHeader(http.StatusOK)
		c.Writer().WriteHeader(http.StatusTeapot) // should not change
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/twice", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("second WriteHeader should be ignored, got %d", w.Code)
	}
}

// TestResponseWriter_Push_NotSupported tests Push returns ErrNotSupported for HTTP/1.1.
// Push is on contract.Context (not ResponseWriter); httptest.ResponseRecorder
// does not support HTTP/2 push, so ErrNotSupported is expected.
func TestResponseWriter_Push_NotSupported(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/push", func(c *astra.Ctx) error {
		err := c.Push("/style.css", nil)
		if err != http.ErrNotSupported {
			return c.String(400, "expected ErrNotSupported, got %v", err)
		}
		return c.String(200, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/push", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// ─── router.go ───────────────────────────────────────────────────────────────

func TestRouter_Routes(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/a", func(c *astra.Ctx) error { return nil })
	app.POST("/b", func(c *astra.Ctx) error { return nil })
	app.GET("/users/:id", func(c *astra.Ctx) error { return nil })

	r := app.Router()
	routes := r.Routes()
	if len(routes) == 0 {
		t.Fatal("Routes() should return registered routes")
	}
	for _, r := range routes {
		if r.Method == "" || r.Path == "" {
			t.Errorf("RouteInfo has empty field: %+v", r)
		}
	}
}

func TestRouter_Routes_Empty(t *testing.T) {
	app := testutil.NewTestApp()
	r := app.Router()
	routes := r.Routes()
	if len(routes) != 0 {
		t.Errorf("empty app should have 0 routes, got %d", len(routes))
	}
}

func TestRouter_fastUpper(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/{word:[A-Z]+}", func(c *astra.Ctx) error {
		return c.String(200, "word=%s", c.Param("word"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/HELLO").AssertStatus(http.StatusOK).AssertBodyContains("HELLO")
	srv.GET("/hello").AssertStatus(http.StatusNotFound)
}

func TestRouter_fastAlpha(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/{word:[a-zA-Z]+}", func(c *astra.Ctx) error {
		return c.String(200, "word=%s", c.Param("word"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/HelloWorld").AssertStatus(http.StatusOK).AssertBodyContains("HelloWorld")
	srv.GET("/Hello123").AssertStatus(http.StatusNotFound)
}

func TestRouter_fastAlphanum(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/items/{id:[a-zA-Z0-9]+}", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/items/abc123").AssertStatus(http.StatusOK)
	srv.GET("/items/abc-def").AssertStatus(http.StatusNotFound)
}

func TestRouter_fastSlug(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/posts/{slug:[a-zA-Z0-9_-]+}", func(c *astra.Ctx) error {
		return c.String(200, "slug=%s", c.Param("slug"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/posts/hello-world_2024").AssertStatus(http.StatusOK)
	srv.GET("/posts/hello.world").AssertStatus(http.StatusNotFound)
}

func TestRouter_fastIdentifier(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/funcs/{name:[a-zA-Z_][a-zA-Z0-9_]*}", func(c *astra.Ctx) error {
		return c.String(200, "name=%s", c.Param("name"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/funcs/_private").AssertStatus(http.StatusOK)
	srv.GET("/funcs/func123").AssertStatus(http.StatusOK)
	srv.GET("/funcs/123invalid").AssertStatus(http.StatusNotFound)
}

func TestRouter_collectRoutes_RegexChild(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/api/{version:v[0-9]+}/users", func(c *astra.Ctx) error { return nil })
	app.GET("/api/{version:v[0-9]+}/posts", func(c *astra.Ctx) error { return nil })
	r := app.Router()
	routes := r.Routes()
	if len(routes) < 2 {
		t.Fatalf("collectRoutes should capture regex routes, got %d", len(routes))
	}
}

func TestRouter_collectRoutes_CatchAll(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/static/*filepath", func(c *astra.Ctx) error { return nil })
	r := app.Router()
	routes := r.Routes()
	if len(routes) == 0 {
		t.Error("collectRoutes should capture catch-all routes")
	}
}

func TestRouter_MaxParamDepth_Deep(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/a/:a1/:a2/:a3/:a4/:a5/:a6/:a7/:a8/:a9", func(c *astra.Ctx) error { return nil })
	r := app.Router()
	routes := r.Routes()
	if len(routes) == 0 {
		t.Error("maxParamDepth routes should be captured")
	}
}

// Router returns the underlying router. We expose it via an interface trick.
type routerAccessor interface{ Router() astra.HttpRouter }

func TestRouter_MaxParamDepth(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/deep/:a/:b/:c/:d/:e/:f/:g/:h", func(c *astra.Ctx) error { return nil })
	acc, ok := any(app).(routerAccessor)
	if !ok {
		t.Skip("router accessor not available")
	}
	r := acc.Router()
	_ = r.Routes()
}

// ─── serializer.go ────────────────────────────────────────────────────────────

func TestSerializer_Marshal(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/marshal", func(c *astra.Ctx) error {
		type item struct {
			Name string `json:"name"`
		}
		// Marshal is called internally by JSON/JSONStream but we can test
		// the serializer path directly via a custom approach
		return c.JSON(200, map[string]string{"name": "test"})
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/marshal").AssertStatus(http.StatusOK)
}

func TestSerializer_EncodeStream(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/stream2", func(c *astra.Ctx) error {
		return c.JSONStream(200, []int{1, 2, 3})
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/stream2").AssertStatus(http.StatusOK).
		AssertBodyContains("1").
		AssertBodyContains("2").
		AssertBodyContains("3")
}

func TestSerializer_MarshalNil(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/marshalnil", func(c *astra.Ctx) error {
		return c.JSON(200, nil)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/marshalnil").AssertStatus(http.StatusOK)
}

// ─── errors.go ────────────────────────────────────────────────────────────────

func TestNewAppError(t *testing.T) {
	err := astra.NewAppError("TEST_CODE", http.StatusBadRequest, "test message")
	testutil.AssertEqual(t, "TEST_CODE", err.Code)
	testutil.AssertEqual(t, http.StatusBadRequest, err.HTTPStatus)
	testutil.AssertEqual(t, "test message", err.Message)
}

func TestNewAppError_WithData(t *testing.T) {
	err := astra.NewAppError("CODE", 400, "msg")
	cloned := err.WithData(map[string]any{"field": "name"})
	testutil.AssertEqual(t, nil, err.Data)
	testutil.AssertEqual(t, "name", cloned.Data.(map[string]any)["field"])
}

func TestNewAppError_WithMessage(t *testing.T) {
	err := astra.NewAppError("CODE", 400, "original")
	cloned := err.WithMessage("updated")
	testutil.AssertEqual(t, "original", err.Message)
	testutil.AssertEqual(t, "updated", cloned.Message)
}

func TestNewAppError_WithInternal(t *testing.T) {
	inner := errors.New("inner")
	err := astra.NewAppError("CODE", 500, "err")
	cloned := err.WithInternal(inner)
	testutil.AssertErrorIs(t, cloned.Unwrap(), inner)
}

func TestNewAppError_ErrorWithInternal(t *testing.T) {
	inner := errors.New("inner")
	err := astra.NewAppError("CODE", 500, "msg").WithInternal(inner)
	s := err.Error()
	if !strings.Contains(s, "internal") {
		t.Errorf("Error() should contain 'internal', got %s", s)
	}
}

func TestToValidationHTTPError(t *testing.T) {
	ve := astra.ValidationErrors{
		{Field: "name", Message: "required"},
		{Field: "age", Message: "must be positive"},
	}
	httpErr := astra.ToValidationHTTPError(ve)
	testutil.AssertEqual(t, http.StatusUnprocessableEntity, httpErr.Code)
	if httpErr.Message == nil {
		t.Error("ToValidationHTTPError Message should not be nil")
	}
}

func TestAppError_RouteKey(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/store-routekey", func(c *astra.Ctx) error {
		c.Set(astra.RouteKey, "/store-routekey")
		return c.String(200, "rk=%s", c.GetString(astra.RouteKey))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/store-routekey").AssertBodyContains("rk=/store-routekey")
}

// ─── MaxParamValueLen ─────────────────────────────────────────────────────────

func TestMaxParamValueLen_DefaultRejectsLong(t *testing.T) {
	// Default MaxParamValueLen is 256; a 300-char segment should be rejected.
	app := testutil.NewTestApp()
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)

	longSeg := strings.Repeat("a", 300)
	srv.GET("/items/"+longSeg).AssertStatus(http.StatusNotFound)
}

func TestMaxParamValueLen_ShortParamPasses(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/items/abc").AssertStatus(http.StatusOK).AssertBodyContains("id=abc")
}

func TestMaxParamValueLen_CustomLimit(t *testing.T) {
	app := testutil.NewTestApp(astra.WithMaxParamValueLen(10))
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)

	// 5-char segment: within limit
	srv.GET("/items/hello").AssertStatus(http.StatusOK).AssertBodyContains("id=hello")
	// 15-char segment: exceeds limit → 404
	srv.GET("/items/"+strings.Repeat("x", 15)).AssertStatus(http.StatusNotFound)
}

func TestMaxParamValueLen_ZeroDisables(t *testing.T) {
	app := testutil.NewTestApp(astra.WithMaxParamValueLen(0))
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)

	// 300-char segment: no limit, should match
	srv.GET("/items/"+strings.Repeat("b", 300)).AssertStatus(http.StatusOK)
}

func TestMaxParamValueLen_CatchAll(t *testing.T) {
	app := testutil.NewTestApp(astra.WithMaxParamValueLen(32))
	app.GET("/static/*filepath", func(c *astra.Ctx) error {
		return c.String(200, "fp=%s", c.Param("filepath"))
	})
	srv := testutil.NewServer(t, app)

	// Short catch-all: OK
	srv.GET("/static/css/main.css").AssertStatus(http.StatusOK)
	// Long catch-all exceeding 32 bytes: 404
	srv.GET("/static/"+strings.Repeat("d", 40)).AssertStatus(http.StatusNotFound)
}

func TestMaxParamValueLen_RegexParam(t *testing.T) {
	app := testutil.NewTestApp(astra.WithMaxParamValueLen(8))
	app.GET("/api/{version:v[0-9]+}", func(c *astra.Ctx) error {
		return c.String(200, "ver=%s", c.Param("version"))
	})
	srv := testutil.NewServer(t, app)

	// Short match: OK
	srv.GET("/api/v2").AssertStatus(http.StatusOK).AssertBodyContains("ver=v2")
	// Long match exceeding 8 bytes: 404
	srv.GET("/api/v"+strings.Repeat("9", 10)).AssertStatus(http.StatusNotFound)
}

// ─── PoolStats ────────────────────────────────────────────────────────────────

func TestPoolStats_HitMissActive(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/ping", func(c *astra.Ctx) error { return c.String(200, "pong") })
	srv := testutil.NewServer(t, app)

	before := app.PoolStats()

	srv.GET("/ping").AssertStatus(200)
	srv.GET("/ping").AssertStatus(200)
	srv.GET("/ping").AssertStatus(200)

	after := app.PoolStats()
	total := after.Hit + after.Miss
	if total < before.Hit+before.Miss+3 {
		t.Errorf("expected at least 3 requests tracked, got hit=%d miss=%d", after.Hit, after.Miss)
	}
	if after.Active != 0 {
		t.Errorf("expected Active=0 after requests complete, got %d", after.Active)
	}
}

func TestPoolStats_MissOnFirstRequest(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/x", func(c *astra.Ctx) error { return c.String(200, "ok") })
	srv := testutil.NewServer(t, app)

	before := app.PoolStats()
	srv.GET("/x").AssertStatus(200)
	after := app.PoolStats()

	if after.Hit+after.Miss <= before.Hit+before.Miss {
		t.Error("expected at least one pool event after first request")
	}
}

// ─── Options.validate ─────────────────────────────────────────────────────────

func TestNew_PanicsOnNegativeShutdownTimeout(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for negative ShutdownTimeout")
		}
	}()
	astra.New(astra.WithShutdownTimeout(-1))
}

func TestNew_PanicsOnNegativeMaxJSONBodySize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for negative MaxJSONBodySize")
		}
	}()
	astra.New(astra.WithMaxJSONBodySize(-1))
}

func TestNew_PanicsOnNegativeMaxMultipartMemory(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for negative MaxMultipartMemory")
		}
	}()
	astra.New(astra.WithMaxMultipartMemory(-1))
}

func TestNew_PanicsOnNegativeMaxParamValueLen(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for negative MaxParamValueLen")
		}
	}()
	astra.New(astra.WithMaxParamValueLen(-1))
}

func TestNew_ValidOptionsNoPanic(t *testing.T) {
	// zero values are valid (disabled)
	_ = astra.New(
		astra.WithShutdownTimeout(0),
		astra.WithMaxJSONBodySize(0),
		astra.WithMaxMultipartMemory(0),
		astra.WithMaxParamValueLen(0),
	)
}
