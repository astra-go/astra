package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/astra-go/astra/mq"
)

const (
	TopicArticlePublished = "article.published"
	TopicCommentCreated   = "comment.created"
	TopicArticleLiked     = "article.liked"
)

type NotificationService struct {
	producer mq.Producer
}

type ArticlePublishedEvent struct {
	ArticleID uint   `json:"article_id"`
	AuthorID  uint   `json:"author_id"`
	Title     string `json:"title"`
}

type CommentCreatedEvent struct {
	CommentID uint   `json:"comment_id"`
	ArticleID uint   `json:"article_id"`
	UserID    uint   `json:"user_id"`
	Content   string `json:"content"`
}

func NewNotificationService(producer mq.Producer) *NotificationService {
	return &NotificationService{producer: producer}
}

func (s *NotificationService) PublishArticlePublished(ctx context.Context, articleID, authorID uint, title string) {
	event := ArticlePublishedEvent{
		ArticleID: articleID,
		AuthorID:  authorID,
		Title:     title,
	}
	s.publish(ctx, TopicArticlePublished, event)
}

func (s *NotificationService) PublishCommentCreated(ctx context.Context, commentID, articleID, userID uint, content string) {
	event := CommentCreatedEvent{
		CommentID: commentID,
		ArticleID: articleID,
		UserID:    userID,
		Content:   content,
	}
	s.publish(ctx, TopicCommentCreated, event)
}

func (s *NotificationService) publish(ctx context.Context, topic string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		slog.Error("notification: marshal event", slog.String("topic", topic), slog.String("err", err.Error()))
		return
	}
	if err := s.producer.Publish(ctx, &mq.Message{
		Topic:   topic,
		Payload: b,
	}); err != nil {
		slog.Error("notification: publish", slog.String("topic", topic), slog.String("err", fmt.Sprintf("%v", err)))
	}
}
