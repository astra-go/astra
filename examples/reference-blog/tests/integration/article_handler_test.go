//go:build integration
// +build integration

// Package integration contains end-to-end HTTP-level integration tests.
//
// Run with:
//
//	make test-integration
//	go test -v -race -tags=integration ./tests/integration/...
//
// Prerequisites: postgres running. Uses TEST_DATABASE_DSN env var if set,
// otherwise defaults to "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable".
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/handler"
	"github.com/astra-go/astra/examples/reference-blog/internal/middleware"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/mq"
	"github.com/astra-go/astra/search/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ── Test Environment ─────────────────────────────────────────────────────────

func getDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable"
}

func setupDB(t *testing.T) *gorm.DB {
	dsn := getDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("skipping: cannot connect to postgres at %s: %v", dsn, err)
	}

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	err = db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{})
	require.NoError(t, err)

	db.Exec("TRUNCATE comments, articles, users CASCADE")
	return db
}

func newTestApp(t *testing.T, db *gorm.DB) *astra.App {
	app := astra.New()

	userRepo := repository.NewUserRepository(db)
	articleRepo := repository.NewArticleRepository(db)
	authSvc := service.NewAuthService(userRepo, "test-jwt-secret", time.Hour)
	notifSvc := service.NewNotificationService(&nullProducer{})
	searchSvc := service.NewSearchService(&nullSearcher{})
	articleSvc := service.NewArticleService(articleRepo, &nullCache{}, notifSvc, searchSvc)
	articleHandler := handler.NewArticleHandler(articleSvc)
	authHandler := handler.NewAuthHandler(authSvc)
	authMw := middleware.NewAuthMiddleware(authSvc)

	app.POST("/api/v1/auth/register", authHandler.Register)
	app.POST("/api/v1/auth/login", authHandler.Login)

	articles := app.Group("/api/v1/articles", authMw.Authenticate)
	articles.POST("", articleHandler.Create)
	articles.GET("", articleHandler.List)
	articles.GET("/:id", articleHandler.GetByID)
	articles.PUT("/:id", articleHandler.Update)
	articles.POST("/:id/publish", articleHandler.Publish)
	articles.DELETE("/:id", articleHandler.Delete)
	articles.POST("/:id/like", articleHandler.Like)

	return app
}

func registerAndLogin(t *testing.T, app *astra.App, username, email, password string) string {
	body, _ := json.Marshal(map[string]string{
		"username": username, "email": email, "password": password,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["token"].(string)
}

// ── Minimal mocks ─────────────────────────────────────────────────────────────

type nullProducer struct{}

func (p *nullProducer) Publish(_ context.Context, _ *mq.Message) error        { return nil }
func (p *nullProducer) PublishBatch(_ context.Context, _ []*mq.Message) error { return nil }
func (p *nullProducer) Close() error                                          { return nil }

type nullSearcher struct{}

func (s *nullSearcher) Index(_ context.Context, _ elastic.IndexRequest) error       { return nil }
func (s *nullSearcher) BulkIndex(_ context.Context, _ []elastic.IndexRequest) error { return nil }
func (s *nullSearcher) Search(_ context.Context, _ elastic.SearchRequest) (*elastic.SearchResult, error) {
	return &elastic.SearchResult{}, nil
}
func (s *nullSearcher) Delete(_, _, _ string) error { return nil }
func (s *nullSearcher) DeleteIndex(_ string) error  { return nil }
func (s *nullSearcher) CreateIndex(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (s *nullSearcher) Close() error { return nil }

type nullCache struct{}

func (c *nullCache) Get(_ context.Context, _ string) ([]byte, error)                  { return nil, cache.ErrCacheMiss }
func (c *nullCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (c *nullCache) Delete(_ context.Context, _ ...string) error                      { return nil }
func (c *nullCache) Exists(_ context.Context, _ string) (bool, error)                 { return false, nil }
func (c *nullCache) Flush(_ context.Context) error                                    { return nil }
func (c *nullCache) Close() error                                                     { return nil }

// ── Test Cases ────────────────────────────────────────────────────────────────

func TestArticleCreateAndPublish(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	token := registerAndLogin(t, app, "author1", "author1@test.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"title":   "My Integration Test Post",
		"content": "This is an integration test article.",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var article map[string]any
	json.Unmarshal(w.Body.Bytes(), &article)
	articleID := int(article["id"].(float64))
	assert.Equal(t, "draft", article["status"])

	// Publish
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/articles/"+strconv.Itoa(articleID)+"/publish", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Verify
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/articles/"+strconv.Itoa(articleID), nil)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	json.Unmarshal(w.Body.Bytes(), &updated)
	assert.Equal(t, "published", updated["status"])
}

func TestArticleList(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	token := registerAndLogin(t, app, "author2", "author2@test.com", "password123")

	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]string{
			"title": "Article " + strconv.Itoa(i), "content": "Content " + strconv.Itoa(i),
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		app.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var article map[string]any
		json.Unmarshal(w.Body.Bytes(), &article)
		id := int(article["id"].(float64))

		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/articles/"+strconv.Itoa(id)+"/publish", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		app.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/articles?page=1&page_size=10", nil)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var pageResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &pageResp)
	assert.Equal(t, float64(3), pageResp["total"])
}

func TestArticleUpdate(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	token := registerAndLogin(t, app, "author3", "author3@test.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"title": "Original Title", "content": "Original content",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var article map[string]any
	json.Unmarshal(w.Body.Bytes(), &article)
	id := int(article["id"].(float64))

	updateBody, _ := json.Marshal(map[string]string{
		"title": "Updated Title", "content": "Updated content",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", "/api/v1/articles/"+strconv.Itoa(id), bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	json.Unmarshal(w.Body.Bytes(), &updated)
	assert.Equal(t, "Updated Title", updated["title"])
	assert.Equal(t, float64(2), updated["version"])
}

func TestArticleDelete(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	token := registerAndLogin(t, app, "author4", "author4@test.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"title": "To Be Deleted", "content": "Content",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var article map[string]any
	json.Unmarshal(w.Body.Bytes(), &article)
	id := int(article["id"].(float64))

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/v1/articles/"+strconv.Itoa(id), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/articles/"+strconv.Itoa(id), nil)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestArticleLike(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	token := registerAndLogin(t, app, "author5", "author5@test.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"title": "Popular Post", "content": "Content",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var article map[string]any
	json.Unmarshal(w.Body.Bytes(), &article)
	id := int(article["id"].(float64))

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/articles/"+strconv.Itoa(id)+"/publish", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	for i := 0; i < 2; i++ {
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/articles/"+strconv.Itoa(id)+"/like", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		app.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/articles/"+strconv.Itoa(id), nil)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var liked map[string]any
	json.Unmarshal(w.Body.Bytes(), &liked)
	assert.Equal(t, float64(2), liked["like_count"])
}

func TestAuthRegisterAndLogin(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	body, _ := json.Marshal(map[string]string{
		"username": "newuser", "email": "newuser@test.com", "password": "password123",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["token"])

	body, _ = json.Marshal(map[string]string{
		"username": "newuser", "password": "password123",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var loginResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &loginResp)
	assert.NotEmpty(t, loginResp["token"])

	body, _ = json.Marshal(map[string]string{
		"username": "newuser", "password": "wrong",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProtectedRoutes(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	body, _ := json.Marshal(map[string]string{
		"title": "Should Fail", "content": "Content",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid-token")
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPagination(t *testing.T) {
	db := setupDB(t)
	app := newTestApp(t, db)

	token := registerAndLogin(t, app, "author6", "author6@test.com", "password123")

	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(map[string]string{
			"title": "Page Test " + strconv.Itoa(i), "content": "Content",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/articles", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		app.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var article map[string]any
		json.Unmarshal(w.Body.Bytes(), &article)
		id := int(article["id"].(float64))

		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/articles/"+strconv.Itoa(id)+"/publish", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		app.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// Page 1, size 2
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/articles?page=1&page_size=2", nil)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var page1 map[string]any
	json.Unmarshal(w.Body.Bytes(), &page1)
	assert.Equal(t, float64(5), page1["total"])
	assert.Len(t, page1["data"].([]any), 2)

	// Page 3 (last page, 1 item)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/articles?page=3&page_size=2", nil)
	app.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var page3 map[string]any
	json.Unmarshal(w.Body.Bytes(), &page3)
	assert.Len(t, page3["data"].([]any), 1)
}
