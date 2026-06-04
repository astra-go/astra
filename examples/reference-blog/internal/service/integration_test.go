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

// setupIntegrationEnv creates a full test environment with real DB
func setupIntegrationEnv(t *testing.T) (*service.AuthService, *service.ArticleService, *service.CommentService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{}))

	userRepo := repository.NewUserRepository(db)
	articleRepo := repository.NewArticleRepository(db)
	commentRepo := repository.NewCommentRepository(db)

	authSvc := service.NewAuthService(userRepo, "integration-secret", 0)
	mockCache := NewMockCache()
	mockProducer := &mockProducer{}
	notifSvc := service.NewNotificationService(mockProducer)
	searchSvc := service.NewSearchService(&mockSearcher{})
	articleSvc := service.NewArticleService(articleRepo, mockCache, notifSvc, searchSvc)
	commentSvc := service.NewCommentService(commentRepo, notifSvc)

	return authSvc, articleSvc, commentSvc, db
}

// TestCompleteBlogFlow tests the full user journey: register → create article → publish → comment → like
func TestCompleteBlogFlow(t *testing.T) {
	authSvc, articleSvc, commentSvc, _ := setupIntegrationEnv(t)
	ctx := context.Background()

	// Step 1: Register user
	authResult, err := authSvc.Register(ctx, service.RegisterRequest{
		Username: "blogger",
		Email:    "blogger@example.com",
		Password: "password123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, authResult.Token)
	userID := authResult.User.ID

	// Step 2: Create article
	article, err := articleSvc.Create(ctx, service.CreateArticleRequest{
		Title:    "My First Blog Post",
		Content:  "This is the content of my first blog post.",
		AuthorID: userID,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleStatusDraft, article.Status)

	// Step 3: Publish article
	err = articleSvc.Publish(ctx, article.ID)
	require.NoError(t, err)

	// Step 4: Read article (should increment view count)
	found, err := articleSvc.GetByID(ctx, article.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleStatusPublished, found.Status)
	assert.Equal(t, "My First Blog Post", found.Title)

	// Step 5: Comment on article
	comment, err := commentSvc.Create(ctx, service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    userID,
		Content:   "Great post!",
	})
	require.NoError(t, err)
	assert.Equal(t, "Great post!", comment.Content)

	// Step 6: Reply to comment
	reply, err := commentSvc.Create(ctx, service.CreateCommentRequest{
		ArticleID: article.ID,
		UserID:    userID,
		ParentID:  &comment.ID,
		Content:   "Thanks for the feedback!",
	})
	require.NoError(t, err)
	assert.Equal(t, comment.ID, *reply.ParentID)

	// Step 7: Like article
	err = articleSvc.Like(ctx, article.ID)
	assert.NoError(t, err)

	// Step 8: Like comment
	err = commentSvc.Like(ctx, comment.ID)
	assert.NoError(t, err)

	// Step 9: List published articles
	articles, total, err := articleSvc.ListPublished(ctx, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, articles, 1)

	// Step 10: List comments for article
	comments, commentTotal, err := commentSvc.ListByArticle(ctx, article.ID, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(2), commentTotal) // original + reply
	assert.Len(t, comments, 2)

	// Step 11: Update article
	updated, err := articleSvc.Update(ctx, service.UpdateArticleRequest{
		ID:      article.ID,
		Title:   "Updated Blog Post",
		Content: "Updated content",
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Blog Post", updated.Title)
	assert.Equal(t, 2, updated.Version)

	// Step 12: Delete comment
	err = commentSvc.Delete(ctx, comment.ID, userID)
	assert.NoError(t, err)

	// Step 13: Delete article
	err = articleSvc.Delete(ctx, article.ID)
	assert.NoError(t, err)

	// Step 14: Verify article is gone
	_, err = articleSvc.GetByID(ctx, article.ID)
	assert.Error(t, err)
}

// TestMultiUserScenario tests interactions between multiple users
func TestMultiUserScenario(t *testing.T) {
	authSvc, articleSvc, commentSvc, _ := setupIntegrationEnv(t)
	ctx := context.Background()

	// Register two users
	author, err := authSvc.Register(ctx, service.RegisterRequest{
		Username: "author", Email: "author@test.com", Password: "password123",
	})
	require.NoError(t, err)

	reader, err := authSvc.Register(ctx, service.RegisterRequest{
		Username: "reader", Email: "reader@test.com", Password: "password123",
	})
	require.NoError(t, err)

	// Author creates and publishes article
	article, err := articleSvc.Create(ctx, service.CreateArticleRequest{
		Title: "Author's Post", Content: "Content", AuthorID: author.User.ID,
	})
	require.NoError(t, err)
	require.NoError(t, articleSvc.Publish(ctx, article.ID))

	// Reader comments
	comment, err := commentSvc.Create(ctx, service.CreateCommentRequest{
		ArticleID: article.ID, UserID: reader.User.ID, Content: "Nice!",
	})
	require.NoError(t, err)

	// Reader cannot delete author's comment (different user)
	// Actually, this is the reader's own comment, so they can delete it
	err = commentSvc.Delete(ctx, comment.ID, reader.User.ID)
	assert.NoError(t, err)

	// Reader cannot delete a comment they didn't write
	authorComment, err := commentSvc.Create(ctx, service.CreateCommentRequest{
		ArticleID: article.ID, UserID: author.User.ID, Content: "Author reply",
	})
	require.NoError(t, err)
	err = commentSvc.Delete(ctx, authorComment.ID, reader.User.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

// TestArticlePublishingWorkflow tests the article lifecycle
func TestArticlePublishingWorkflow(t *testing.T) {
	authSvc, articleSvc, _, _ := setupIntegrationEnv(t)
	ctx := context.Background()

	user, err := authSvc.Register(ctx, service.RegisterRequest{
		Username: "writer", Email: "writer@test.com", Password: "password123",
	})
	require.NoError(t, err)

	// Create draft
	draft, err := articleSvc.Create(ctx, service.CreateArticleRequest{
		Title: "Draft Post", Content: "Draft content", AuthorID: user.User.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleStatusDraft, draft.Status)

	// Publish
	require.NoError(t, articleSvc.Publish(ctx, draft.ID))
	published, err := articleSvc.GetByID(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ArticleStatusPublished, published.Status)
	assert.NotNil(t, published.PublishedAt)

	// Archive (update status to archived)
	// Currently the service doesn't have Archive method, so skip this step

	// List only published
	_, total, err := articleSvc.ListPublished(ctx, orm.Page{PageNum: 1, PageSize: 10, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

// TestAuthenticationFlow tests login/logout scenarios
func TestAuthenticationFlow(t *testing.T) {
	authSvc, _, _, _ := setupIntegrationEnv(t)
	ctx := context.Background()

	// Register
	regResult, err := authSvc.Register(ctx, service.RegisterRequest{
		Username: "testuser", Email: "test@test.com", Password: "password123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, regResult.Token)

	// Login with correct credentials
	loginResult, err := authSvc.Login(ctx, service.LoginRequest{
		Username: "testuser", Password: "password123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, loginResult.Token)

	// Validate token
	claims, err := authSvc.ValidateToken(loginResult.Token)
	require.NoError(t, err)
	assert.Equal(t, "testuser", claims.Username)

	// Wrong password
	_, err = authSvc.Login(ctx, service.LoginRequest{
		Username: "testuser", Password: "wrong",
	})
	assert.Equal(t, "invalid credentials", err.Error())
}
