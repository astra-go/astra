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

func setupCommentTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{}))
	return db
}

func createCommentTestData(db *gorm.DB) (*domain.User, *domain.Article) {
	user := &domain.User{Username: "author", Email: "a@b.com", PasswordHash: "hash", Role: "reader"}
	db.Create(user)
	article := &domain.Article{Title: "Test", Content: "Content", AuthorID: user.ID, Status: domain.ArticleStatusPublished}
	db.Create(article)
	return user, article
}

func TestCommentRepository_Create(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := repository.NewCommentRepository(db)
	_, article := createCommentTestData(db)

	comment := &domain.Comment{
		ArticleID: article.ID,
		UserID:    1,
		Content:   "Great article!",
	}

	err := repo.Create(context.Background(), comment)
	assert.NoError(t, err)
	assert.NotZero(t, comment.ID)
}

func TestCommentRepository_FindByID(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := repository.NewCommentRepository(db)
	_, article := createCommentTestData(db)

	comment := &domain.Comment{ArticleID: article.ID, UserID: 1, Content: "Comment"}
	require.NoError(t, repo.Create(context.Background(), comment))

	found, err := repo.FindByID(context.Background(), comment.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Comment", found.Content)
}

func TestCommentRepository_FindByArticle(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := repository.NewCommentRepository(db)
	_, article := createCommentTestData(db)

	for i := 0; i < 5; i++ {
		comment := &domain.Comment{ArticleID: article.ID, UserID: 1, Content: "Comment"}
		require.NoError(t, repo.Create(context.Background(), comment))
	}

	comments, total, err := repo.FindByArticle(context.Background(), article.ID, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})
	assert.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, comments, 5)
}

func TestCommentRepository_IncrLikeCount(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := repository.NewCommentRepository(db)
	_, article := createCommentTestData(db)

	comment := &domain.Comment{ArticleID: article.ID, UserID: 1, Content: "Likeable"}
	require.NoError(t, repo.Create(context.Background(), comment))

	err := repo.IncrLikeCount(context.Background(), comment.ID)
	assert.NoError(t, err)
}

func TestCommentRepository_Delete(t *testing.T) {
	db := setupCommentTestDB(t)
	repo := repository.NewCommentRepository(db)
	_, article := createCommentTestData(db)

	comment := &domain.Comment{ArticleID: article.ID, UserID: 1, Content: "To delete"}
	require.NoError(t, repo.Create(context.Background(), comment))

	err := repo.Delete(context.Background(), comment.ID)
	assert.NoError(t, err)

	_, err = repo.FindByID(context.Background(), comment.ID)
	assert.Error(t, err)
}
