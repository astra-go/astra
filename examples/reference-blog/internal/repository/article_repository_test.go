package repository_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/orm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupArticleTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.Article{}))
	return db
}

func createTestUser(db *gorm.DB) *domain.User {
	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "author"}
	db.Create(user)
	return user
}

func TestArticleRepository_Create(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{
		Title:    "Test Article",
		Content:  "Content",
		AuthorID: user.ID,
		Status:   domain.ArticleStatusDraft,
	}

	err := repo.Create(context.Background(), article)
	assert.NoError(t, err)
	assert.NotZero(t, article.ID)
}

func TestArticleRepository_FindByID(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), article))

	found, err := repo.FindByID(context.Background(), article.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Test", found.Title)
	assert.NotNil(t, found.Author) // Preloaded
}

func TestArticleRepository_FindByID_NotFound(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)

	found, err := repo.FindByID(context.Background(), 999)
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestArticleRepository_FindPublished(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	// Create published and draft articles
	published := &domain.Article{Title: "Published", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	draft := &domain.Article{Title: "Draft", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), published))
	require.NoError(t, repo.Create(context.Background(), draft))

	articles, total, err := repo.FindPublished(context.Background(), orm.Page{PageNum: 1, PageSize: 10, Offset: 0})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, articles, 1)
	assert.Equal(t, "Published", articles[0].Title)
}

func TestArticleRepository_FindByAuthor(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	for i := 0; i < 3; i++ {
		article := &domain.Article{Title: "Article", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
		require.NoError(t, repo.Create(context.Background(), article))
	}

	articles, total, err := repo.FindByAuthor(context.Background(), user.ID, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, articles, 3)
}

func TestArticleRepository_Update(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{Title: "Original", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), article))

	article.Title = "Updated"
	err := repo.Update(context.Background(), article)
	assert.NoError(t, err)

	found, _ := repo.FindByID(context.Background(), article.ID)
	assert.Equal(t, "Updated", found.Title)
}

func TestArticleRepository_UpdateStatus(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), article))

	err := repo.UpdateStatus(context.Background(), article.ID, domain.ArticleStatusPublished)
	assert.NoError(t, err)
}

func TestArticleRepository_IncrViewCount(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), article))

	err := repo.IncrViewCount(context.Background(), article.ID)
	assert.NoError(t, err)
}

func TestArticleRepository_IncrLikeCount(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), article))

	err := repo.IncrLikeCount(context.Background(), article.ID)
	assert.NoError(t, err)
}

func TestArticleRepository_Delete(t *testing.T) {
	db := setupArticleTestDB(t)
	repo := repository.NewArticleRepository(db)
	user := createTestUser(db)

	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusDraft}
	require.NoError(t, repo.Create(context.Background(), article))

	err := repo.Delete(context.Background(), article.ID)
	assert.NoError(t, err)

	// Soft delete — FindByID should fail
	_, err = repo.FindByID(context.Background(), article.ID)
	assert.Error(t, err)
}
