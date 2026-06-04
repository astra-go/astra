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

func setupArticleServiceTest(t *testing.T) (*service.ArticleService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{}))

	articleRepo := repository.NewArticleRepository(db)
	mockCache := NewMockCache()
	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	searchSvc := service.NewSearchService(&mockSearcher{})

	svc := service.NewArticleService(articleRepo, mockCache, notifSvc, searchSvc)
	return svc, db
}

func TestArticleService_Create(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	// Create a user
	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	article, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Test Article",
		Content:  "Article content here",
		AuthorID: user.ID,
	})

	assert.NoError(t, err)
	assert.NotNil(t, article)
	assert.Equal(t, "Test Article", article.Title)
	assert.Equal(t, "Article content here", article.Content)
	assert.Equal(t, domain.ArticleStatusDraft, article.Status)
	assert.Equal(t, user.ID, article.AuthorID)
}

func TestArticleService_Create_WithSummary(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	summary := "A brief summary"
	tags := "golang,testing"
	article, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Test Article",
		Content:  "Content",
		Summary:  &summary,
		Tags:     &tags,
		AuthorID: user.ID,
	})

	assert.NoError(t, err)
	assert.NotNil(t, article)
	assert.Equal(t, &summary, article.Summary)
	assert.Equal(t, &tags, article.Tags)
}

func TestArticleService_GetByID(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	created, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Test Article",
		Content:  "Content",
		AuthorID: user.ID,
	})
	require.NoError(t, err)

	found, err := svc.GetByID(context.Background(), created.ID)

	assert.NoError(t, err)
	assert.Equal(t, created.Title, found.Title)
}

func TestArticleService_GetByID_NotFound(t *testing.T) {
	svc, _ := setupArticleServiceTest(t)

	_, err := svc.GetByID(context.Background(), 999)

	assert.Error(t, err)
}

func TestArticleService_Update(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	created, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Original Title",
		Content:  "Original Content",
		AuthorID: user.ID,
	})
	require.NoError(t, err)

	updated, err := svc.Update(context.Background(), service.UpdateArticleRequest{
		ID:      created.ID,
		Title:   "Updated Title",
		Content: "Updated Content",
	})

	assert.NoError(t, err)
	assert.Equal(t, "Updated Title", updated.Title)
	assert.Equal(t, "Updated Content", updated.Content)
	assert.Equal(t, 2, updated.Version) // version incremented
}

func TestArticleService_Publish(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	created, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Draft Article",
		Content:  "Content",
		AuthorID: user.ID,
	})
	require.NoError(t, err)

	err = svc.Publish(context.Background(), created.ID)

	assert.NoError(t, err)

	// Verify status changed
	found, _ := svc.GetByID(context.Background(), created.ID)
	assert.Equal(t, domain.ArticleStatusPublished, found.Status)
}

func TestArticleService_Delete(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	created, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "To Delete",
		Content:  "Content",
		AuthorID: user.ID,
	})
	require.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	assert.NoError(t, err)

	// Verify article is gone (soft delete)
	_, err = svc.GetByID(context.Background(), created.ID)
	assert.Error(t, err)
}

func TestArticleService_Like(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	created, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Likeable",
		Content:  "Content",
		AuthorID: user.ID,
	})
	require.NoError(t, err)

	err = svc.Like(context.Background(), created.ID)
	assert.NoError(t, err)
}

func TestArticleService_ListPublished(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)

	// Create 3 articles, publish 2
	for i := 0; i < 3; i++ {
		article, err := svc.Create(context.Background(), service.CreateArticleRequest{
			Title:    "Article",
			Content:  "Content",
			AuthorID: user.ID,
		})
		require.NoError(t, err)
		if i < 2 {
			err = svc.Publish(context.Background(), article.ID)
			require.NoError(t, err)
		}
	}

	articles, total, err := svc.ListPublished(context.Background(), orm.Page{PageNum: 1, PageSize: 10, Offset: 0})

	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, articles, 2)
}

func TestArticleService_ListByAuthor(t *testing.T) {
	svc, db := setupArticleServiceTest(t)

	user1 := &domain.User{Username: "author1", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	user2 := &domain.User{Username: "author2", Email: "c@d.com", PasswordHash: "hash", Role: "author"}
	db.Create(user1)
	db.Create(user2)

	for i := 0; i < 3; i++ {
		_, err := svc.Create(context.Background(), service.CreateArticleRequest{
			Title:    "Article",
			Content:  "Content",
			AuthorID: user1.ID,
		})
		require.NoError(t, err)
	}
	_, err := svc.Create(context.Background(), service.CreateArticleRequest{
		Title:    "Other Article",
		Content:  "Content",
		AuthorID: user2.ID,
	})
	require.NoError(t, err)

	articles, total, err := svc.ListByAuthor(context.Background(), user1.ID, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})

	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, articles, 3)
}
