//go:build integration

package search_test

// testcontainers-based integration tests for search/elastic.
//
// Two modes are supported:
//
//  1. CI mode — set ELASTICSEARCH_URL to point at an externally managed container:
//     ELASTICSEARCH_URL=http://localhost:9200 \
//       go test -tags integration -v ./e2e/search/...
//
//  2. Local-dev mode (testcontainers) — no env-var needed; Docker must be available:
//     go test -tags integration -v ./e2e/search/...

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	estc "github.com/testcontainers/testcontainers-go/modules/elasticsearch"

	"github.com/astra-go/astra/search/elastic"
)

var (
	containerAddr   string
	containerUser   string
	containerPass   string
	containerCACert []byte
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// CI mode: use externally managed Elasticsearch.
	if addr := os.Getenv("ELASTICSEARCH_URL"); addr != "" {
		containerAddr = addr
		os.Exit(m.Run())
	}

	// Local-dev mode: spin up a container via testcontainers.
	ctr, err := estc.Run(ctx,
		"docker.elastic.co/elasticsearch/elasticsearch:8.13.0",
		estc.WithPassword("testpass"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers: start Elasticsearch: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	containerAddr = ctr.Settings.Address
	containerUser = ctr.Settings.Username
	containerPass = ctr.Settings.Password
	containerCACert = ctr.Settings.CACert

	os.Exit(m.Run())
}

func newClient(t *testing.T) *elastic.Client {
	t.Helper()
	cfg := elastic.Config{
		Addresses: []string{containerAddr},
		Username:  containerUser,
		Password:  containerPass,
	}
	if len(containerCACert) > 0 {
		cfg.CACert = containerCACert
	}
	c, err := elastic.New(cfg)
	if err != nil {
		t.Fatalf("elastic.New: %v", err)
	}
	return c
}

func uniqueIndex(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("astra-test-%d", rand.Int63())
}

func withIndex(t *testing.T, c *elastic.Client, mapping map[string]any) string {
	t.Helper()
	ctx := context.Background()
	idx := uniqueIndex(t)
	if err := c.CreateIndex(ctx, idx, mapping); err != nil {
		t.Fatalf("CreateIndex(%s): %v", idx, err)
	}
	t.Cleanup(func() { _ = c.DeleteIndex(ctx, idx) })
	return idx
}

func refresh() { time.Sleep(1500 * time.Millisecond) }

// ─── Lifecycle ────────────────────────────────────────────────────────────────

func TestIntegration_CreateAndDeleteIndex(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := uniqueIndex(t)

	if err := c.CreateIndex(ctx, idx, nil); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	if err := c.DeleteIndex(ctx, idx); err != nil {
		t.Fatalf("DeleteIndex: %v", err)
	}
}

func TestIntegration_CreateIndex_WithMapping(t *testing.T) {
	c := newClient(t)
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"sku":   map[string]any{"type": "keyword"},
				"price": map[string]any{"type": "float"},
			},
		},
	}
	idx := withIndex(t, c, mapping)
	err := c.CreateIndex(context.Background(), idx, nil)
	if err == nil {
		t.Error("expected error when creating a duplicate index")
	}
}

// ─── Index / Search round-trip ────────────────────────────────────────────────

func TestIntegration_IndexAndSearch(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"name":  map[string]any{"type": "keyword"},
				"price": map[string]any{"type": "float"},
			},
		},
	})

	if err := c.Index(ctx, elastic.IndexRequest{
		Index: idx,
		ID:    "prod-001",
		Doc:   map[string]any{"name": "Widget", "price": 9.99},
	}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	refresh()

	result, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"term": map[string]any{"name": "Widget"}},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected Total=1, got %d", result.Total)
	}
	if result.Hits[0].ID != "prod-001" {
		t.Errorf("expected ID=prod-001, got %s", result.Hits[0].ID)
	}
	if result.Hits[0].Source["name"] != "Widget" {
		t.Errorf("unexpected source: %v", result.Hits[0].Source)
	}
}

func TestIntegration_Index_Overwrite(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, nil)

	for _, v := range []string{"v1", "v2"} {
		if err := c.Index(ctx, elastic.IndexRequest{
			Index: idx, ID: "doc-1", Doc: map[string]any{"val": v},
		}); err != nil {
			t.Fatalf("Index(%s): %v", v, err)
		}
	}
	refresh()

	res, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"ids": map[string]any{"values": []string{"doc-1"}}},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("expected 1 doc after overwrite, got %d", res.Total)
	}
	if res.Hits[0].Source["val"] != "v2" {
		t.Errorf("expected val=v2 after overwrite, got %v", res.Hits[0].Source["val"])
	}
}

// ─── BulkIndex ────────────────────────────────────────────────────────────────

func TestIntegration_BulkIndex(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, nil)

	const n = 10
	reqs := make([]elastic.IndexRequest, n)
	for i := range reqs {
		reqs[i] = elastic.IndexRequest{
			Index: idx,
			ID:    fmt.Sprintf("doc-%02d", i),
			Doc:   map[string]any{"seq": i},
		}
	}
	if err := c.BulkIndex(ctx, reqs); err != nil {
		t.Fatalf("BulkIndex: %v", err)
	}
	refresh()

	result, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"match_all": map[string]any{}},
		Size:  20,
	})
	if err != nil {
		t.Fatalf("Search after BulkIndex: %v", err)
	}
	if result.Total != n {
		t.Errorf("expected %d docs, got %d", n, result.Total)
	}
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestIntegration_Delete(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, nil)

	if err := c.Index(ctx, elastic.IndexRequest{Index: idx, ID: "del-1", Doc: map[string]any{"x": 1}}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	refresh()

	if err := c.Delete(ctx, idx, "del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	refresh()

	res, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"ids": map[string]any{"values": []string{"del-1"}}},
	})
	if err != nil {
		t.Fatalf("Search after Delete: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("expected 0 docs after delete, got %d", res.Total)
	}
}

func TestIntegration_Delete_NonExistent(t *testing.T) {
	c := newClient(t)
	idx := withIndex(t, c, nil)
	err := c.Delete(context.Background(), idx, "ghost-doc")
	if err != nil {
		t.Errorf("Delete of non-existent doc should not error, got: %v", err)
	}
}

// ─── Pagination ───────────────────────────────────────────────────────────────

func TestIntegration_Search_Pagination(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, nil)

	for i := 0; i < 15; i++ {
		if err := c.Index(ctx, elastic.IndexRequest{
			Index: idx, ID: fmt.Sprintf("p-%02d", i), Doc: map[string]any{"n": i},
		}); err != nil {
			t.Fatalf("Index: %v", err)
		}
	}
	refresh()

	page1, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"match_all": map[string]any{}},
		Size:  5, From: 0,
	})
	if err != nil {
		t.Fatalf("Search page1: %v", err)
	}
	if len(page1.Hits) != 5 {
		t.Errorf("page1: expected 5 hits, got %d", len(page1.Hits))
	}
	if page1.Total != 15 {
		t.Errorf("page1: expected Total=15, got %d", page1.Total)
	}

	page2, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"match_all": map[string]any{}},
		Size:  5, From: 5,
	})
	if err != nil {
		t.Fatalf("Search page2: %v", err)
	}
	if len(page2.Hits) != 5 {
		t.Errorf("page2: expected 5 hits, got %d", len(page2.Hits))
	}
	ids1 := make(map[string]bool)
	for _, h := range page1.Hits {
		ids1[h.ID] = true
	}
	for _, h := range page2.Hits {
		if ids1[h.ID] {
			t.Errorf("ID %s appears in both page1 and page2", h.ID)
		}
	}
}

// ─── Aggregations ─────────────────────────────────────────────────────────────

func TestIntegration_Search_Aggregation(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"category": map[string]any{"type": "keyword"},
			},
		},
	})

	for _, d := range []struct{ id, cat string }{
		{"a1", "books"}, {"a2", "books"}, {"a3", "tools"},
	} {
		if err := c.Index(ctx, elastic.IndexRequest{
			Index: idx, ID: d.id, Doc: map[string]any{"category": d.cat},
		}); err != nil {
			t.Fatalf("Index: %v", err)
		}
	}
	refresh()

	result, err := c.Search(ctx, elastic.SearchRequest{
		Index: []string{idx},
		Query: map[string]any{"match_all": map[string]any{}},
		Size:  0,
		Aggs: map[string]any{
			"by_category": map[string]any{
				"terms": map[string]any{"field": "category"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Search with aggs: %v", err)
	}
	if result.Aggs == nil {
		t.Fatal("expected aggregations in result")
	}
	byCat, ok := result.Aggs["by_category"]
	if !ok {
		t.Fatal("expected by_category aggregation")
	}
	buckets, _ := byCat.(map[string]any)["buckets"].([]any)
	if len(buckets) != 2 {
		t.Errorf("expected 2 buckets (books, tools), got %d", len(buckets))
	}
}

// ─── Source filtering ─────────────────────────────────────────────────────────

func TestIntegration_Search_SourceFilter(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	idx := withIndex(t, c, nil)

	if err := c.Index(ctx, elastic.IndexRequest{
		Index: idx, ID: "sf-1",
		Doc:   map[string]any{"name": "Gadget", "price": 19.99, "stock": 42},
	}); err != nil {
		t.Fatalf("Index: %v", err)
	}
	refresh()

	res, err := c.Search(ctx, elastic.SearchRequest{
		Index:  []string{idx},
		Query:  map[string]any{"match_all": map[string]any{}},
		Source: []string{"name", "price"},
	})
	if err != nil {
		t.Fatalf("Search with source filter: %v", err)
	}
	if res.Total != 1 {
		t.Fatalf("expected 1 hit, got %d", res.Total)
	}
	src := res.Hits[0].Source
	if _, ok := src["name"]; !ok {
		t.Error("expected 'name' field in filtered source")
	}
	if _, ok := src["stock"]; ok {
		t.Error("'stock' should have been excluded by source filter")
	}
}
