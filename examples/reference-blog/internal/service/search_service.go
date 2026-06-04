package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/search/elastic"
)

const articleIndex = "articles"

type SearchService struct {
	searcher elastic.Searcher
}

type ArticleDoc struct {
	ID      uint   `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Content string `json:"content"`
	Tags    string `json:"tags"`
	Author  string `json:"author"`
}

type SearchResult struct {
	Total    int64
	Articles []ArticleDoc
}

func NewSearchService(searcher elastic.Searcher) *SearchService {
	return &SearchService{searcher: searcher}
}

func (s *SearchService) IndexArticle(ctx context.Context, article *domain.Article) error {
	doc := ArticleDoc{
		ID:    article.ID,
		Title: article.Title,
	}
	if article.Summary != nil {
		doc.Summary = *article.Summary
	}
	if article.Tags != nil {
		doc.Tags = *article.Tags
	}
	if article.Author != nil {
		doc.Author = article.Author.Username
	}

	return s.searcher.Index(ctx, elastic.IndexRequest{
		Index: articleIndex,
		ID:    strconv.FormatUint(uint64(article.ID), 10),
		Doc:   doc,
	})
}

func (s *SearchService) DeleteArticle(ctx context.Context, articleID uint) error {
	return s.searcher.Delete(ctx, articleIndex, strconv.FormatUint(uint64(articleID), 10))
}

func (s *SearchService) Search(ctx context.Context, query string, page, pageSize int) (*SearchResult, error) {
	from := (page - 1) * pageSize
	if from < 0 {
		from = 0
	}

	result, err := s.searcher.Search(ctx, elastic.SearchRequest{
		Index: []string{articleIndex},
		Query: map[string]any{
			"multi_match": map[string]any{
				"query":  query,
				"fields": []string{"title^3", "summary^2", "content", "tags^2", "author"},
			},
		},
		Size: pageSize,
		From: from,
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	sr := &SearchResult{Total: result.Total}
	for _, hit := range result.Hits {
		doc := ArticleDoc{}
		if v, ok := hit.Source["id"]; ok {
			if f, ok := v.(float64); ok {
				doc.ID = uint(f)
			}
		}
		if v, ok := hit.Source["title"].(string); ok {
			doc.Title = v
		}
		if v, ok := hit.Source["summary"].(string); ok {
			doc.Summary = v
		}
		if v, ok := hit.Source["author"].(string); ok {
			doc.Author = v
		}
		sr.Articles = append(sr.Articles, doc)
	}
	return sr, nil
}

func (s *SearchService) EnsureIndex(ctx context.Context) error {
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"title":   map[string]any{"type": "text", "analyzer": "standard"},
				"summary": map[string]any{"type": "text"},
				"content": map[string]any{"type": "text"},
				"tags":    map[string]any{"type": "text"},
				"author":  map[string]any{"type": "keyword"},
			},
		},
	}
	return s.searcher.CreateIndex(ctx, articleIndex, mapping)
}
