package service_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/orm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupArticleTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{}))
	return db
}

func TestCommentService_Create(t *testing.T) {
	db := setupArticleTestDB(t)
	commentRepo := repository.NewCommentRepository(db)

	// Create test user and article
	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)

	// NotificationService needs mq.Producer - use mock
	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	svc := service.NewCommentService(commentRepo, notifSvc)

	comment, err := svc.Create(context.Background(), service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    user.ID,
		Content:   "Great article!",
	})

	assert.NoError(t, err)
	assert.NotNil(t, comment)
	assert.Equal(t, "Great article!", comment.Content)
	assert.Equal(t, article.ID, comment.ArticleID)
	assert.Equal(t, user.ID, comment.UserID)
}

func TestCommentService_Create_WithParent(t *testing.T) {
	db := setupArticleTestDB(t)
	commentRepo := repository.NewCommentRepository(db)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)

	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	svc := service.NewCommentService(commentRepo, notifSvc)

	// Create parent comment
	parent, err := svc.Create(context.Background(), service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    user.ID,
		Content:   "Parent comment",
	})
	require.NoError(t, err)

	// Create reply
	reply, err := svc.Create(context.Background(), service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    user.ID,
		ParentID:  &parent.ID,
		Content:   "Reply comment",
	})

	assert.NoError(t, err)
	assert.NotNil(t, reply)
	assert.Equal(t, parent.ID, *reply.ParentID)
}

func TestCommentService_ListByArticle(t *testing.T) {
	db := setupArticleTestDB(t)
	commentRepo := repository.NewCommentRepository(db)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)

	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	svc := service.NewCommentService(commentRepo, notifSvc)

	// Create 3 comments
	for i := 0; i < 3; i++ {
		_, err := svc.Create(context.Background(), service.CreateCommentRequest{
			ArticleID: article.ID,
			UserID:    user.ID,
			Content:   "Comment",
		})
		require.NoError(t, err)
	}

	comments, total, err := svc.ListByArticle(context.Background(), article.ID, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})

	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, comments, 3)
}

func TestCommentService_Delete_Success(t *testing.T) {
	db := setupArticleTestDB(t)
	commentRepo := repository.NewCommentRepository(db)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)

	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	svc := service.NewCommentService(commentRepo, notifSvc)

	comment, err := svc.Create(context.Background(), service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    user.ID,
		Content:   "To be deleted",
	})
	require.NoError(t, err)

	err = svc.Delete(context.Background(), comment.ID, user.ID)
	assert.NoError(t, err)
}

func TestCommentService_Delete_Unauthorized(t *testing.T) {
	db := setupArticleTestDB(t)
	commentRepo := repository.NewCommentRepository(db)

	user1 := &domain.User{Username: "user1", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	user2 := &domain.User{Username: "user2", Email: "c@d.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user1)
	db.Create(user2)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user1.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)

	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	svc := service.NewCommentService(commentRepo, notifSvc)

	comment, err := svc.Create(context.Background(), service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    user1.ID,
		Content:   "My comment",
	})
	require.NoError(t, err)

	// user2 tries to delete user1's comment
	err = svc.Delete(context.Background(), comment.ID, user2.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestCommentService_Like(t *testing.T) {
	db := setupArticleTestDB(t)
	commentRepo := repository.NewCommentRepository(db)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)

	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	svc := service.NewCommentService(commentRepo, notifSvc)

	comment, err := svc.Create(context.Background(), service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    user.ID,
		Content:   "Likeable comment",
	})
	require.NoError(t, err)

	err = svc.Like(context.Background(), comment.ID)
	assert.NoError(t, err)
}
