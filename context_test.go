package astra_test

import (
	"encoding/json"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/astra-go/astra/testutil"
)

// ─── Context response methods ────────────────────────────────────────────────

func TestCtx_JSON_Response(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/json", func(c *astra.Ctx) error {
		return c.JSON(200, map[string]string{"hello": "world"})
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/json")
	resp.AssertStatus(http.StatusOK).
		AssertHeader("Content-Type", "application/json; charset=utf-8").
		AssertBodyContains(`"hello"`).
		AssertBodyContains(`"world"`)
}

func TestCtx_JSON_Created(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/create", func(c *astra.Ctx) error {
		return c.JSON(http.StatusCreated, map[string]int{"id": 42})
	})
	srv := testutil.NewServer(t, app)

	resp := srv.POST("/create", nil)
	resp.AssertStatus(http.StatusCreated).
		AssertBodyContains(`"id"`)
}

func TestCtx_String_Response(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/string", func(c *astra.Ctx) error {
		return c.String(200, "hello %s", "world")
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/string")
	resp.AssertStatus(http.StatusOK).
		AssertBodyContains("hello world")
}

func TestCtx_HTML_Response(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/html", func(c *astra.Ctx) error {
		return c.HTML(200, "<h1>Hello</h1>")
	})
	srv := testutil.NewServer(t, app)

	resp := srv.GET("/html")
	resp.AssertStatus(http.StatusOK).
		AssertHeader("Content-Type", "text/html; charset=utf-8").
		AssertBodyContains("<h1>Hello</h1>")
}

func TestCtx_NoContent(t *testing.T) {
	app := testutil.NewTestApp()
	app.DELETE("/resource", func(c *astra.Ctx) error {
		return c.NoContent(http.StatusNoContent)
	})
	srv := testutil.NewServer(t, app)

	resp := srv.DELETE("/resource")
	resp.AssertStatus(http.StatusNoContent)
}

func TestCtx_Redirect(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/redirect", func(c *astra.Ctx) error {
		return c.Redirect(http.StatusFound, "/target")
	})
	app.GET("/target", func(c *astra.Ctx) error {
		return c.String(200, "target")
	})
	srv := testutil.NewServer(t, app)

	// Use raw http.Client to follow redirect
	client := srv.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Get(srv.URL() + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("want %d, got %d", http.StatusFound, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/target" {
		t.Errorf("want Location /target, got %s", loc)
	}
}

// ─── Context request methods ────────────────────────────────────────────────

func TestCtx_Param(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/:id", func(c *astra.Ctx) error {
		return c.String(200, "user-%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/users/42").AssertBodyContains("user-42")
}

func TestCtx_Query(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/search", func(c *astra.Ctx) error {
		return c.String(200, "q=%s", c.Query("q"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/search?q=golang").AssertBodyContains("q=golang")
}

func TestCtx_DefaultQuery(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/search", func(c *astra.Ctx) error {
		return c.String(200, "q=%s", c.DefaultQuery("q", "default"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/search").AssertBodyContains("q=default")
	srv.GET("/search?q=custom").AssertBodyContains("q=custom")
}

func TestCtx_Header(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/header", func(c *astra.Ctx) error {
		return c.String(200, "ua=%s", c.Header("X-Custom"))
	})
	srv := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL()+"/header", nil)
	req.Header.Set("X-Custom", "test-value")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestCtx_SetHeader(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/set-header", func(c *astra.Ctx) error {
		c.SetHeader("X-Response", "hello")
		return c.String(200, "ok")
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/set-header").AssertHeader("X-Response", "hello")
}

// ─── Context store (Set/Get) ────────────────────────────────────────────────

func TestCtx_SetGet(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/store", func(c *astra.Ctx) error {
		c.Set("key", "value")
		val, ok := c.Get("key")
		if !ok {
			return c.String(500, "not found")
		}
		return c.String(200, "%v", val)
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/store").AssertBodyContains("value")
}

func TestCtx_MustGet_Panics(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Recovery())
	app.GET("/mustget", func(c *astra.Ctx) error {
		val := c.MustGet("nonexistent")
		return c.String(200, "%v", val)
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/mustget").AssertStatus(http.StatusInternalServerError)
}

func TestCtx_GetString(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/getstring", func(c *astra.Ctx) error {
		c.Set("name", "astra")
		return c.String(200, "name=%s", c.GetString("name"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/getstring").AssertBodyContains("name=astra")
}

func TestCtx_GetInt(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/getint", func(c *astra.Ctx) error {
		c.Set("count", 42)
		return c.String(200, "count=%d", c.GetInt("count"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/getint").AssertBodyContains("count=42")
}

func TestCtx_Set_Overwrite(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/overwrite", func(c *astra.Ctx) error {
		c.Set("key", "first")
		c.Set("key", "second")
		return c.String(200, "%v", c.GetString("key"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/overwrite").AssertBodyContains("second")
}

// ─── Context BindJSON ────────────────────────────────────────────────────────

func TestCtx_BindJSON(t *testing.T) {
	type input struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	app := testutil.NewTestApp()
	app.POST("/bind", func(c *astra.Ctx) error {
		var in input
		if err := c.BindJSON(&in); err != nil {
			return c.String(400, "bind error: %v", err)
		}
		return c.JSON(200, in)
	})
	srv := testutil.NewServer(t, app)

	body, _ := json.Marshal(input{Name: "test", Age: 25})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL()+"/bind", strings.NewReader(string(body)))
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

// ─── Multiple params in one path ─────────────────────────────────────────────

func TestCtx_MultipleParams(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/:userId/posts/:postId", func(c *astra.Ctx) error {
		return c.String(200, "user=%s,post=%s", c.Param("userId"), c.Param("postId"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/users/7/posts/42").AssertBodyContains("user=7,post=42")
}

// ─── Wildcard param ──────────────────────────────────────────────────────────

func TestCtx_WildcardParam(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/files/*path", func(c *astra.Ctx) error {
		return c.String(200, "path=%s", c.Param("path"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/files/docs/readme.md").AssertBodyContains("path=/docs/readme.md")
}

// ─── Blob response ───────────────────────────────────────────────────────────

func TestCtx_Blob(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/blob", func(c *astra.Ctx) error {
		return c.Blob(200, "application/octet-stream", []byte("raw bytes"))
	})
	srv := testutil.NewServer(t, app)

	srv.GET("/blob").
		AssertStatus(http.StatusOK).
		AssertHeader("Content-Type", "application/octet-stream")
}

// ─── Ctx Writer status ───────────────────────────────────────────────────────

func TestCtx_Writer_Status(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/status", func(c *astra.Ctx) error {
		c.Writer().WriteHeader(http.StatusTeapot)
		return nil
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	app.ServeHTTP(resp, req)

	if resp.Code != http.StatusTeapot {
		t.Errorf("want %d, got %d", http.StatusTeapot, resp.Code)
	}
}

// ─── kvStore adaptive map promotion ─────────────────────────────────────────

func TestCtx_KvStore_MapPromotion(t *testing.T) {
	// Fill the store beyond the threshold; kvMap should be populated.
	app := testutil.NewTestApp()
	app.GET("/promote", func(c *astra.Ctx) error {
		for i := range astra.KvStoreMapThreshold + 1 {
			c.Set(fmt.Sprintf("key%d", i), i)
		}
		// All keys must be readable after promotion.
		for i := range astra.KvStoreMapThreshold + 1 {
			v, ok := c.Get(fmt.Sprintf("key%d", i))
			if !ok {
				return c.String(500, "key%d missing", i)
			}
			if v.(int) != i {
				return c.String(500, "key%d wrong value", i)
			}
		}
		if astra.CtxKvMap(c) == nil {
			return c.String(500, "kvMap not promoted")
		}
		return c.String(200, "ok")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/promote").AssertStatus(200).AssertBodyContains("ok")
}

func TestCtx_KvStore_MapPromotion_Overwrite(t *testing.T) {
	// Overwriting a key after promotion must update both slice and map.
	app := testutil.NewTestApp()
	app.GET("/overwrite-promoted", func(c *astra.Ctx) error {
		for i := range astra.KvStoreMapThreshold + 1 {
			c.Set(fmt.Sprintf("key%d", i), i)
		}
		c.Set("key0", 999)
		v, _ := c.Get("key0")
		if v.(int) != 999 {
			return c.String(500, "overwrite failed: got %v", v)
		}
		return c.String(200, "ok")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/overwrite-promoted").AssertStatus(200).AssertBodyContains("ok")
}

func TestCtx_KvStore_ResetClearsMap(t *testing.T) {
	// After a request that promoted to map mode, the next request must start
	// with an empty store (map cleared, not nil — allocation is retained).
	app := testutil.NewTestApp()
	count := 0
	app.GET("/reset-map", func(c *astra.Ctx) error {
		count++
		if count == 1 {
			for i := range astra.KvStoreMapThreshold + 1 {
				c.Set(fmt.Sprintf("key%d", i), i)
			}
			return c.String(200, "first")
		}
		// Second request: map should be empty (cleared by reset).
		if m := astra.CtxKvMap(c); m != nil && len(m) != 0 {
			return c.String(500, "map not cleared: len=%d", len(m))
		}
		_, ok := c.Get("key0")
		if ok {
			return c.String(500, "key0 leaked across requests")
		}
		return c.String(200, "clean")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/reset-map").AssertBodyContains("first")
	srv.GET("/reset-map").AssertStatus(200).AssertBodyContains("clean")
}
