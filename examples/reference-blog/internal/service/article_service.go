package service

import (
	"context"
	"fmt"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/timeutil"
)

type ArticleService struct {
	repo         *repository.ArticleRepository
	cache        cache.Cache
	notification *NotificationService
	search       *SearchService
	cacheTTL     time.Duration
}

type CreateArticleRequest struct {
	Title    string
	Content  string
	Summary  *string
	Tags     *string
	AuthorID uint
}

type UpdateArticleRequest struct {
	ID      uint
	Title   string
	Content string
	Summary *string
	Tags    *string
}

func NewArticleService(
	repo *repository.ArticleRepository,
	cache cache.Cache,
	notification *NotificationService,
	search *SearchService,
) *ArticleService {
	return &ArticleService{
		repo:         repo,
		cache:        cache,
		notification: notification,
		search:       search,
		cacheTTL:     15 * time.Minute,
	}
}

func (s *ArticleService) Create(ctx context.Context, req CreateArticleRequest) (*domain.Article, error) {
	article := &domain.Article{
		Title:    req.Title,
		Content:  req.Content,
		Summary:  req.Summary,
		Tags:     req.Tags,
		AuthorID: req.AuthorID,
		Status:   domain.ArticleStatusDraft,
	}
	if err := s.repo.Create(ctx, article); err != nil {
		return nil, fmt.Errorf("create article: %w", err)
	}
	return article, nil
}

func (s *ArticleService) GetByID(ctx context.Context, id uint) (*domain.Article, error) {
	cacheKey := fmt.Sprintf("article:%d", id)

	var article domain.Article
	if err := cache.GetJSON(ctx, s.cache, cacheKey, &article); err == nil {
		return &article, nil
	}

	dbArticle, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find article: %w", err)
	}

	_ = cache.SetJSON(ctx, s.cache, cacheKey, dbArticle, s.cacheTTL)

	go func() {
		_ = s.repo.IncrViewCount(context.Background(), id)
	}()

	return dbArticle, nil
}

func (s *ArticleService) ListPublished(ctx context.Context, page orm.Page) ([]*domain.Article, int64, error) {
	return s.repo.FindPublished(ctx, page)
}

func (s *ArticleService) ListByAuthor(ctx context.Context, authorID uint, page orm.Page) ([]*domain.Article, int64, error) {
	return s.repo.FindByAuthor(ctx, authorID, page)
}

func (s *ArticleService) Update(ctx context.Context, req UpdateArticleRequest) (*domain.Article, error) {
	article, err := s.repo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("find article: %w", err)
	}

	article.Title = req.Title
	article.Content = req.Content
	article.Summary = req.Summary
	article.Tags = req.Tags
	article.Version++

	if err := s.repo.Update(ctx, article); err != nil {
		return nil, fmt.Errorf("update article: %w", err)
	}

	s.invalidateCache(ctx, article.ID)

	return article, nil
}

func (s *ArticleService) Publish(ctx context.Context, id uint) error {
	article, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find article: %w", err)
	}

	now := timeutil.Now()
	article.PublishedAt = &now
	article.Status = domain.ArticleStatusPublished

	if err := s.repo.UpdateStatus(ctx, id, domain.ArticleStatusPublished); err != nil {
		return fmt.Errorf("publish article: %w", err)
	}

	s.invalidateCache(ctx, id)

	go func() {
		s.notification.PublishArticlePublished(context.Background(), article.ID, article.AuthorID, article.Title)
		_ = s.search.IndexArticle(context.Background(), article)
	}()

	return nil
}

func (s *ArticleService) Delete(ctx context.Context, id uint) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete article: %w", err)
	}

	s.invalidateCache(ctx, id)

	go func() {
		_ = s.search.DeleteArticle(context.Background(), id)
	}()

	return nil
}

func (s *ArticleService) Like(ctx context.Context, id uint) error {
	if err := s.repo.IncrLikeCount(ctx, id); err != nil {
		return fmt.Errorf("like article: %w", err)
	}

	s.invalidateCache(ctx, id)
	return nil
}

func (s *ArticleService) invalidateCache(ctx context.Context, id uint) {
	cacheKey := fmt.Sprintf("article:%d", id)
	_ = s.cache.Delete(ctx, cacheKey)
}
