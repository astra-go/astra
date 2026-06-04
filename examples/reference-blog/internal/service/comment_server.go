package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/orm"
)

// CommentServiceServer wraps CommentService for gRPC server use.
type CommentServiceServer struct {
	repo         *repository.CommentRepository
	notification *NotificationService
}

type CreateCommentServerRequest struct {
	ArticleID uint
	UserID    uint
	ParentID  *uint
	Content   string
}

type CommentServerResponse struct {
	ID        uint
	ArticleID uint
	UserID    uint
	ParentID  *uint
	Content   string
	LikeCount int64
	CreatedAt string
	UpdatedAt string
}

func NewCommentServiceServer(repo *repository.CommentRepository, notification *NotificationService) *CommentServiceServer {
	return &CommentServiceServer{repo: repo, notification: notification}
}

func (s *CommentServiceServer) Create(ctx context.Context, req CreateCommentServerRequest) (*CommentServerResponse, error) {
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
		if s.notification != nil {
			s.notification.PublishCommentCreated(context.Background(), comment.ID, comment.ArticleID, comment.UserID, comment.Content)
		}
	}()

	return toCommentResponse(comment), nil
}

func (s *CommentServiceServer) ListByArticle(ctx context.Context, articleID uint, page orm.Page) ([]*CommentServerResponse, int64, error) {
	comments, total, err := s.repo.FindByArticle(ctx, articleID, page)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*CommentServerResponse, 0, len(comments))
	for _, c := range comments {
		responses = append(responses, toCommentResponse(c))
	}
	return responses, total, nil
}

func (s *CommentServiceServer) Delete(ctx context.Context, id, userID uint) error {
	comment, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find comment: %w", err)
	}
	if comment == nil {
		return errors.New("comment not found")
	}
	if comment.UserID != userID {
		return errors.New("unauthorized: comment belongs to another user")
	}
	return s.repo.Delete(ctx, id)
}

func (s *CommentServiceServer) Like(ctx context.Context, id uint) (*CommentServerResponse, error) {
	if err := s.repo.IncrLikeCount(ctx, id); err != nil {
		return nil, fmt.Errorf("like comment: %w", err)
	}
	comment, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toCommentResponse(comment), nil
}

func toCommentResponse(c *domain.Comment) *CommentServerResponse {
	return &CommentServerResponse{
		ID:        c.ID,
		ArticleID: c.ArticleID,
		UserID:    c.UserID,
		ParentID:  c.ParentID,
		Content:   c.Content,
		LikeCount: c.LikeCount,
	}
}
