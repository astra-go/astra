package service

import (
	"context"
	"fmt"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/orm"
)

type CommentService struct {
	repo         *repository.CommentRepository
	notification *NotificationService
}

type CreateCommentRequest struct {
	ArticleID uint
	UserID    uint
	ParentID  *uint
	Content   string
}

func NewCommentService(repo *repository.CommentRepository, notification *NotificationService) *CommentService {
	return &CommentService{
		repo:         repo,
		notification: notification,
	}
}

func (s *CommentService) Create(ctx context.Context, req CreateCommentRequest) (*domain.Comment, error) {
	comment := &domain.Comment{
		ArticleID: req.ArticleID,
		UserID:    req.UserID,
		ParentID:  req.ParentID,
		Content:   req.Content,
	}
	if err := s.repo.Create(ctx, comment); err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}

	go func() {
		s.notification.PublishCommentCreated(context.Background(), comment.ID, comment.ArticleID, comment.UserID, comment.Content)
	}()

	return comment, nil
}

func (s *CommentService) ListByArticle(ctx context.Context, articleID uint, page orm.Page) ([]*domain.Comment, int64, error) {
	return s.repo.FindByArticle(ctx, articleID, page)
}

func (s *CommentService) Delete(ctx context.Context, id, userID uint) error {
	comment, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find comment: %w", err)
	}
	if comment.UserID != userID {
		return fmt.Errorf("unauthorized: comment belongs to another user")
	}
	return s.repo.Delete(ctx, id)
}

func (s *CommentService) Like(ctx context.Context, id uint) error {
	return s.repo.IncrLikeCount(ctx, id)
}
