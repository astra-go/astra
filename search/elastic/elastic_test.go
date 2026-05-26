package elastic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astra-go/astra/search/elastic"
	"github.com/astra-go/astra/testutil"
)

// ─── Compile-time interface check ────────────────────────────────────────────

var _ elastic.Searcher = (*elastic.Client)(nil)

// ─── Client construction ──────────────────────────────────────────────────────

func TestNew_DefaultConfig_Succeeds(t *testing.T) {
	_, err := elastic.New(elastic.Config{})
	testutil.AssertNoError(t, err)
}

func TestNew_InsecureSkipVerify_Succeeds(t *testing.T) {
	_, err := elastic.New(elastic.Config{InsecureSkipVerify: true})
	testutil.AssertNoError(t, err)
}

func TestNew_WithAddresses_Succeeds(t *testing.T) {
	_, err := elastic.New(elastic.Config{
		Addresses: []string{"http://localhost:9200"},
	})
	testutil.AssertNoError(t, err)
}

// ─── Close ────────────────────────────────────────────────────────────────────

func TestClient_Close_ReturnsNil(t *testing.T) {
	c, err := elastic.New(elastic.Config{})
	testutil.AssertNoError(t, err)
	testutil.AssertNoError(t, c.Close())
}

// ─── BulkIndex — empty input early return ────────────────────────────────────

func TestClient_BulkIndex_EmptySlice_ReturnsNil(t *testing.T) {
	c, _ := elastic.New(elastic.Config{})
	err := c.BulkIndex(context.Background(), nil)
	testutil.AssertNoError(t, err)

	err = c.BulkIndex(context.Background(), []elastic.IndexRequest{})
	testutil.AssertNoError(t, err)
}

// ─── Mock-server tests ────────────────────────────────────────────────────────

// mockESServer creates an httptest.Server that handles a subset of the
// Elasticsearch REST API used by elastic.Client.
//
// The ES Go client v8 performs a product check on every response — it looks
// for the X-Elastic-Product header with value "Elasticsearch".  Our mock must
// inject that header on every response or the client rejects it.
func mockESServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// setESHeaders injects the headers required to pass the ES client's
	// product-check validation.
	setESHeaders := func(w http.ResponseWriter) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
	}

	// Index: PUT /index/_doc/id  or  POST /index/_doc
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setESHeaders(w)
		switch {
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/_doc/"):
			json.NewEncoder(w).Encode(map[string]any{"result": "created", "_id": "1"})

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_doc"):
			json.NewEncoder(w).Encode(map[string]any{"result": "created", "_id": "auto"})

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_bulk"):
			json.NewEncoder(w).Encode(map[string]any{"errors": false, "items": []any{}})

		case strings.HasSuffix(r.URL.Path, "/_search"):
			json.NewEncoder(w).Encode(map[string]any{
				"hits": map[string]any{
					"total": map[string]any{"value": 1, "relation": "eq"},
					"hits": []any{
						map[string]any{
							"_index":  "test",
							"_id":     "1",
							"_score":  1.0,
							"_source": map[string]any{"name": "widget"},
						},
					},
				},
			})

		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/_doc/"):
			json.NewEncoder(w).Encode(map[string]any{"result": "deleted"})

		case r.Method == http.MethodDelete:
			// DeleteIndex
			json.NewEncoder(w).Encode(map[string]any{"acknowledged": true})

		case r.Method == http.MethodPut:
			// CreateIndex
			json.NewEncoder(w).Encode(map[string]any{"acknowledged": true})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newMockClient(t *testing.T) *elastic.Client {
	t.Helper()
	srv := mockESServer(t)
	c, err := elastic.New(elastic.Config{Addresses: []string{srv.URL}})
	testutil.AssertNoError(t, err)
	return c
}

func TestClient_Index_Succeeds(t *testing.T) {
	c := newMockClient(t)
	err := c.Index(context.Background(), elastic.IndexRequest{
		Index: "test",
		ID:    "1",
		Doc:   map[string]any{"name": "widget"},
	})
	testutil.AssertNoError(t, err)
}

func TestClient_BulkIndex_Succeeds(t *testing.T) {
	c := newMockClient(t)
	err := c.BulkIndex(context.Background(), []elastic.IndexRequest{
		{Index: "test", ID: "1", Doc: map[string]any{"a": 1}},
		{Index: "test", ID: "2", Doc: map[string]any{"a": 2}},
	})
	testutil.AssertNoError(t, err)
}

func TestClient_Search_ReturnsHits(t *testing.T) {
	c := newMockClient(t)
	res, err := c.Search(context.Background(), elastic.SearchRequest{
		Index: []string{"test"},
		Query: map[string]any{"match_all": map[string]any{}},
	})
	testutil.AssertNoError(t, err)
	if res.Total != 1 {
		t.Errorf("expected Total=1, got %d", res.Total)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(res.Hits))
	}
	testutil.AssertEqual(t, "test", res.Hits[0].Index)
	testutil.AssertEqual(t, "1", res.Hits[0].ID)
}

func TestClient_Delete_Succeeds(t *testing.T) {
	c := newMockClient(t)
	err := c.Delete(context.Background(), "test", "1")
	testutil.AssertNoError(t, err)
}

func TestClient_DeleteIndex_Succeeds(t *testing.T) {
	c := newMockClient(t)
	err := c.DeleteIndex(context.Background(), "test")
	testutil.AssertNoError(t, err)
}

func TestClient_CreateIndex_NoMapping_Succeeds(t *testing.T) {
	c := newMockClient(t)
	err := c.CreateIndex(context.Background(), "test", nil)
	testutil.AssertNoError(t, err)
}

func TestClient_CreateIndex_WithMapping_Succeeds(t *testing.T) {
	c := newMockClient(t)
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"name": map[string]any{"type": "keyword"},
			},
		},
	}
	err := c.CreateIndex(context.Background(), "test", mapping)
	testutil.AssertNoError(t, err)
}

// TestClient_Index_ServerError verifies that a 4xx/5xx response becomes an error.
func TestClient_Index_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c, _ := elastic.New(elastic.Config{Addresses: []string{srv.URL}})
	err := c.Index(context.Background(), elastic.IndexRequest{Index: "x", ID: "1", Doc: "doc"})
	if err == nil {
		t.Error("expected error from 500 response")
	}
}
