// Package elastic provides a unified Elasticsearch / OpenSearch client for Astra.
//
// It wraps github.com/elastic/go-elasticsearch/v8 and exposes a minimal,
// ergonomic interface covering the most common search operations:
// indexing, bulk indexing, searching, and index management.
//
// The Client also works with AWS OpenSearch Service and self-hosted
// OpenSearch — both are wire-compatible with the Elasticsearch 7/8 API.
//
// # Quick start
//
//	import "github.com/astra-go/astra/search/elastic"
//
//	client, err := elastic.New(elastic.Config{
//	    Addresses: []string{"http://localhost:9200"},
//	})
//
//	// Index a document
//	client.Index(ctx, elastic.IndexRequest{
//	    Index: "products",
//	    ID:    "prod-001",
//	    Doc:   map[string]any{"name": "Widget", "price": 9.99},
//	})
//
//	// Search
//	result, err := client.Search(ctx, elastic.SearchRequest{
//	    Index: []string{"products"},
//	    Query: map[string]any{
//	        "match": map[string]any{"name": "widget"},
//	    },
//	    Size: 10,
//	})
//
// # Authentication
//
//	// Basic auth
//	client, _ := elastic.New(elastic.Config{
//	    Addresses: []string{"https://my-es:9200"},
//	    Username:  "elastic",
//	    Password:  os.Getenv("ES_PASSWORD"),
//	})
//
//	// API key
//	client, _ := elastic.New(elastic.Config{
//	    Addresses: []string{"https://my-es:9200"},
//	    APIKey:    os.Getenv("ES_API_KEY"),
//	})
//
//	// Elastic Cloud
//	client, _ := elastic.New(elastic.Config{
//	    CloudID:  os.Getenv("ELASTIC_CLOUD_ID"),
//	    APIKey:   os.Getenv("ELASTIC_API_KEY"),
//	})
package elastic

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	esv8 "github.com/elastic/go-elasticsearch/v8"
)

// Searcher is the unified Elasticsearch / OpenSearch interface.
type Searcher interface {
	// Index creates or replaces a single document.
	Index(ctx context.Context, req IndexRequest) error

	// BulkIndex indexes multiple documents in a single API call.
	BulkIndex(ctx context.Context, reqs []IndexRequest) error

	// Search executes a query and returns matching documents.
	Search(ctx context.Context, req SearchRequest) (*SearchResult, error)

	// Delete removes a single document by ID.
	Delete(ctx context.Context, index, id string) error

	// DeleteIndex removes an entire index and all its documents.
	DeleteIndex(ctx context.Context, index string) error

	// CreateIndex creates a new index with an optional mapping.
	// mapping may be nil for a default mapping.
	CreateIndex(ctx context.Context, index string, mapping map[string]any) error

	// Close is a no-op for the HTTP-based client but satisfies resource-release
	// semantics expected by callers.
	Close() error
}

// Config configures the Elasticsearch client.
type Config struct {
	// Addresses is the list of Elasticsearch node URLs.
	// Example: []string{"http://localhost:9200"}
	Addresses []string

	// Username and Password for HTTP basic authentication.
	Username string
	Password string

	// APIKey for API key authentication (base64-encoded "id:api_key").
	APIKey string

	// CloudID for Elastic Cloud deployments.
	CloudID string

	// CACert is the PEM-encoded CA certificate for TLS verification.
	CACert []byte

	// InsecureSkipVerify disables TLS certificate verification.
	//
	// WARNING: only for development/testing environments. Enabling this in
	// production exposes the connection to man-in-the-middle attacks.
	// Use CACert to trust a custom CA instead.
	InsecureSkipVerify bool
}

// IndexRequest describes a document to be indexed.
type IndexRequest struct {
	// Index is the target index name.
	Index string

	// ID is the document ID. When empty, Elasticsearch auto-generates one.
	ID string

	// Doc is the document body. Must be JSON-serialisable.
	Doc any

	// Pipeline is the name of an ingest pipeline to run on the document.
	Pipeline string
}

// SearchRequest describes a search query.
type SearchRequest struct {
	// Index is the list of indices to search. Wildcards are supported.
	Index []string

	// Query is the raw Elasticsearch query DSL as a map.
	Query map[string]any

	// Size is the maximum number of hits to return. Default: 10.
	Size int

	// From is the starting offset for pagination. Default: 0.
	From int

	// Sort is a list of sort specifications.
	// Example: []map[string]any{{"timestamp": "desc"}}
	Sort []map[string]any

	// Source lists the fields to include in each hit's _source.
	// Empty means include all fields.
	Source []string

	// Aggs contains aggregation definitions (pass-through to ES).
	Aggs map[string]any
}

// SearchResult contains the results of a Search call.
type SearchResult struct {
	// Total is the total number of matching documents.
	Total int64

	// Hits contains the returned document hits.
	Hits []Hit

	// Aggs contains aggregation results (raw from Elasticsearch).
	Aggs map[string]any
}

// Hit is a single search result document.
type Hit struct {
	// Index is the index the document belongs to.
	Index string

	// ID is the document ID.
	ID string

	// Score is the relevance score (0 for non-scored queries).
	Score float64

	// Source contains the document fields.
	Source map[string]any
}

// Client implements Searcher.
type Client struct {
	es *esv8.Client
}

// New creates an Elasticsearch Client.
func New(cfg Config) (*Client, error) {
	esCfg := esv8.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
		APIKey:    cfg.APIKey,
		CloudID:   cfg.CloudID,
	}
	if len(cfg.CACert) > 0 {
		esCfg.CACert = cfg.CACert
	}
	if cfg.InsecureSkipVerify {
		slog.Warn("elastic: InsecureSkipVerify is enabled — TLS certificate verification is disabled; use only in development/testing environments")
		esCfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}

	es, err := esv8.NewClient(esCfg)
	if err != nil {
		return nil, fmt.Errorf("elastic: new client: %w", err)
	}
	return &Client{es: es}, nil
}

// Index creates or replaces a document.
func (c *Client) Index(ctx context.Context, req IndexRequest) error {
	body, err := json.Marshal(req.Doc)
	if err != nil {
		return fmt.Errorf("elastic: marshal doc: %w", err)
	}

	apiReq := c.es.Index
	resp, err := apiReq(req.Index, bytes.NewReader(body),
		c.es.Index.WithContext(ctx),
		c.es.Index.WithDocumentID(req.ID),
		c.es.Index.WithPipeline(req.Pipeline),
	)
	if err != nil {
		return fmt.Errorf("elastic: index %s/%s: %w", req.Index, req.ID, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("elastic: index %s/%s: %s", req.Index, req.ID, resp.Status())
	}
	return nil
}

// BulkIndex indexes multiple documents using the Bulk API.
func (c *Client) BulkIndex(ctx context.Context, reqs []IndexRequest) error {
	if len(reqs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, req := range reqs {
		meta := map[string]any{
			"index": map[string]any{
				"_index": req.Index,
				"_id":    req.ID,
			},
		}
		if err := json.NewEncoder(&buf).Encode(meta); err != nil {
			return fmt.Errorf("elastic: bulk meta: %w", err)
		}
		if err := json.NewEncoder(&buf).Encode(req.Doc); err != nil {
			return fmt.Errorf("elastic: bulk doc: %w", err)
		}
	}

	resp, err := c.es.Bulk(bytes.NewReader(buf.Bytes()),
		c.es.Bulk.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("elastic: bulk: %w", err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("elastic: bulk: %s", resp.Status())
	}
	return nil
}

// Search executes a query.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	body := map[string]any{}
	if req.Query != nil {
		body["query"] = req.Query
	}
	size := req.Size
	if size <= 0 {
		size = 10
	}
	body["size"] = size
	body["from"] = req.From
	if len(req.Sort) > 0 {
		body["sort"] = req.Sort
	}
	if len(req.Source) > 0 {
		body["_source"] = req.Source
	}
	if len(req.Aggs) > 0 {
		body["aggs"] = req.Aggs
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elastic: marshal search: %w", err)
	}

	resp, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(req.Index...),
		c.es.Search.WithBody(bytes.NewReader(encoded)),
	)
	if err != nil {
		return nil, fmt.Errorf("elastic: search: %w", err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return nil, fmt.Errorf("elastic: search: %s", resp.Status())
	}

	return parseSearchResponse(resp.Body)
}

// Delete removes a single document.
func (c *Client) Delete(ctx context.Context, index, id string) error {
	resp, err := c.es.Delete(index, id, c.es.Delete.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("elastic: delete %s/%s: %w", index, id, err)
	}
	defer resp.Body.Close()
	if resp.IsError() && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("elastic: delete %s/%s: %s", index, id, resp.Status())
	}
	return nil
}

// DeleteIndex removes an index.
func (c *Client) DeleteIndex(ctx context.Context, index string) error {
	resp, err := c.es.Indices.Delete([]string{index}, c.es.Indices.Delete.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("elastic: delete index %s: %w", index, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("elastic: delete index %s: %s", index, resp.Status())
	}
	return nil
}

// CreateIndex creates an index with an optional mapping.
func (c *Client) CreateIndex(ctx context.Context, index string, mapping map[string]any) error {
	var body io.Reader
	if mapping != nil {
		data, err := json.Marshal(mapping)
		if err != nil {
			return fmt.Errorf("elastic: marshal mapping: %w", err)
		}
		body = bytes.NewReader(data)
	}

	resp, err := c.es.Indices.Create(index,
		c.es.Indices.Create.WithContext(ctx),
		c.es.Indices.Create.WithBody(body),
	)
	if err != nil {
		return fmt.Errorf("elastic: create index %s: %w", index, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		return fmt.Errorf("elastic: create index %s: %s", index, resp.Status())
	}
	return nil
}

// Close is a no-op (the HTTP transport is managed by the ES client).
func (c *Client) Close() error { return nil }

// Compile-time assertion.
var _ Searcher = (*Client)(nil)

// ─── response parsing ─────────────────────────────────────────────────────────

func parseSearchResponse(r io.Reader) (*SearchResult, error) {
	var raw struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Index  string         `json:"_index"`
				ID     string         `json:"_id"`
				Score  float64        `json:"_score"`
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
		Aggregations map[string]any `json:"aggregations"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("elastic: decode response: %w", err)
	}

	result := &SearchResult{
		Total: raw.Hits.Total.Value,
		Aggs:  raw.Aggregations,
	}
	for _, h := range raw.Hits.Hits {
		result.Hits = append(result.Hits, Hit{
			Index:  h.Index,
			ID:     h.ID,
			Score:  h.Score,
			Source: h.Source,
		})
	}
	return result, nil
}
