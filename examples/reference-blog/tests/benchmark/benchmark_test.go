//go:build bench
// +build bench

// Package benchmark contains performance benchmark tests.
//
// Run with:
//
//	make benchmark
//	go test -bench=. -benchmem -benchtime=3s ./tests/benchmark/...
//
// Uses SQLite in-memory for consistent, fast I/O.
// Benchmarks target the service layer (repository + service).
package benchmark_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/search/elastic"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ── Fixtures ──────────────────────────────────────────────────────────────────

func setupBenchmarkDB(tb testing.TB) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		tb.Fatalf("open sqlite: %v", err)
	}

	db.Exec("PRAGMA synchronous = OFF")
	db.Exec("PRAGMA journal_mode = MEMORY")
	db.Exec("PRAGMA cache_size = 10000")

	err = db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{})
	if err != nil {
		tb.Fatalf("migrate: %v", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	return db
}

func seedArticles(tb testing.TB, db *gorm.DB, count int) []uint {
	db.Create(&domain.User{Username: "bench1", Email: "bench1@test.com", PasswordHash: "hash"})
	db.Create(&domain.User{Username: "bench2", Email: "bench2@test.com", PasswordHash: "hash"})

	ids := make([]uint, count)
	for i := 0; i < count; i++ {
		a := domain.Article{
			Title:    fmt.Sprintf("Benchmark Article %d", i),
			Content:  "Benchmark content for performance testing. " + fmt.Sprintf("Item %d", i),
			AuthorID: 1,
			Status:   domain.ArticleStatusPublished,
		}
		db.Create(&a)
		ids[i] = a.ID
	}
	return ids
}

func newArticleService(tb testing.TB, db *gorm.DB) *service.ArticleService {
	articleRepo := repository.NewArticleRepository(db)
	authRepo := repository.NewUserRepository(db)
	authSvc := service.NewAuthService(authRepo, "bench-secret", time.Hour)
	notifSvc := service.NewNotificationService(&noopProducer{})
	searchSvc := service.NewSearchService(&noopSearcher{})
	return service.NewArticleService(articleRepo, &noopCache{}, notifSvc, searchSvc)
}

// ── Mocks ─────────────────────────────────────────────────────────────────────

type noopProducer struct{}

func (p *noopProducer) Publish(_ context.Context, _ any) error        { return nil }
func (p *noopProducer) PublishBatch(_ context.Context, _ []any) error { return nil }
func (p *noopProducer) Close() error                                  { return nil }

type noopSearcher struct{}

func (s *noopSearcher) Index(_ context.Context, _ elastic.IndexRequest) error       { return nil }
func (s *noopSearcher) BulkIndex(_ context.Context, _ []elastic.IndexRequest) error { return nil }
func (s *noopSearcher) Search(_ context.Context, _ elastic.SearchRequest) (*elastic.SearchResult, error) {
	return &elastic.SearchResult{}, nil
}
func (s *noopSearcher) Delete(_ context.Context, _, _ string) error   { return nil }
func (s *noopSearcher) DeleteIndex(_ context.Context, _ string) error { return nil }
func (s *noopSearcher) CreateIndex(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (s *noopSearcher) Close() error { return nil }

type noopCache struct{}

func (c *noopCache) Get(_ context.Context, _ string) ([]byte, error)                  { return nil, cache.ErrCacheMiss }
func (c *noopCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (c *noopCache) Delete(_ context.Context, _ ...string) error                      { return nil }
func (c *noopCache) Exists(_ context.Context, _ string) (bool, error)                 { return false, nil }
func (c *noopCache) Flush(_ context.Context) error                                    { return nil }
func (c *noopCache) Close() error                                                     { return nil }

// ── Benchmarks ────────────────────────────────────────────────────────────────

// BenchmarkArticleCreate measures article creation throughput.
func BenchmarkArticleCreate(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)

	user := domain.User{Username: "benchuser", Email: "bench@test.com", PasswordHash: "hash"}
	db.Create(&user)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = svc.Create(ctx, service.CreateArticleRequest{
			Title:    fmt.Sprintf("Article %d", i),
			Content:  "Benchmark content for article creation test.",
			AuthorID: user.ID,
		})
	}
}

// BenchmarkArticlePublish measures the publish workflow.
func BenchmarkArticlePublish(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)

	user := domain.User{Username: "benchuser2", Email: "bench2@test.com", PasswordHash: "hash"}
	db.Create(&user)

	ctx := context.Background()
	ids := make([]uint, b.N)
	for i := 0; i < b.N; i++ {
		a, _ := svc.Create(ctx, service.CreateArticleRequest{
			Title:    fmt.Sprintf("To Publish %d", i),
			Content:  "Content",
			AuthorID: user.ID,
		})
		ids[i] = a.ID
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = svc.Publish(ctx, ids[i])
	}
}

// BenchmarkArticleGetByID measures article retrieval (cache miss path).
func BenchmarkArticleGetByID(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	ids := seedArticles(b, db, 1000)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = svc.GetByID(ctx, ids[i%1000])
	}
}

// BenchmarkArticleListPublished measures paginated published article listing.
func BenchmarkArticleListPublished(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	seedArticles(b, db, 1000)

	ctx := context.Background()
	page := orm.Page{PageNum: 1, PageSize: 20, Offset: 0}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = svc.ListPublished(ctx, page)
	}
}

// BenchmarkArticleListPublished_DeepPage measures deep pagination (offset=100).
func BenchmarkArticleListPublished_DeepPage(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	seedArticles(b, db, 1000)

	ctx := context.Background()
	page := orm.Page{PageNum: 6, PageSize: 20, Offset: 100}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = svc.ListPublished(ctx, page)
	}
}

// BenchmarkArticleUpdate measures optimistic-lock update.
func BenchmarkArticleUpdate(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	ids := seedArticles(b, db, 100)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = svc.Update(ctx, service.UpdateArticleRequest{
			ID:      ids[i%100],
			Title:   fmt.Sprintf("Updated Title %d", i),
			Content: fmt.Sprintf("Updated content %d", i),
		})
	}
}

// BenchmarkArticleLike measures atomic like count increment.
func BenchmarkArticleLike(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	ids := seedArticles(b, db, 100)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = svc.Like(ctx, ids[i%100])
	}
}

// BenchmarkArticleDelete measures soft delete performance.
func BenchmarkArticleDelete(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)

	ctx := context.Background()
	ids := make([]uint, b.N)
	for i := 0; i < b.N; i++ {
		a := domain.Article{
			Title:    fmt.Sprintf("To Delete %d", i),
			Content:  "Content",
			AuthorID: 1,
			Status:   domain.ArticleStatusPublished,
		}
		db.Create(&a)
		ids[i] = a.ID
	}

	b.ResetTimer()
	b.ReportAllocs()

	for _, id := range ids {
		_ = svc.Delete(ctx, id)
	}
}

// BenchmarkAuthRegister measures user registration (includes bcrypt).
func BenchmarkAuthRegister(b *testing.B) {
	db := setupBenchmarkDB(b)
	authRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(authRepo, "bench-secret", time.Hour)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = svc.Register(ctx, service.RegisterRequest{
			Username: fmt.Sprintf("benchuser%d", i),
			Email:    fmt.Sprintf("bench%d@test.com", i),
			Password: "password123",
		})
	}
}

// BenchmarkAuthLogin measures login with bcrypt comparison.
func BenchmarkAuthLogin(b *testing.B) {
	db := setupBenchmarkDB(b)
	authRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(authRepo, "bench-secret", time.Hour)

	ctx := context.Background()
	_, _ = svc.Register(ctx, service.RegisterRequest{
		Username: "benchlogin", Email: "benchlogin@test.com", Password: "password123",
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = svc.Login(ctx, service.LoginRequest{
			Username: "benchlogin", Password: "password123",
		})
	}
}

// BenchmarkAuthValidateToken measures JWT token validation.
func BenchmarkAuthValidateToken(b *testing.B) {
	db := setupBenchmarkDB(b)
	authRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(authRepo, "bench-secret", time.Hour)

	ctx := context.Background()
	result, _ := svc.Register(ctx, service.RegisterRequest{
		Username: "tokentest", Email: "tokentest@test.com", Password: "password123",
	})
	token := result.Token

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = svc.ValidateToken(token)
	}
}

// BenchmarkConcurrentReads measures parallel article reads.
func BenchmarkConcurrentReads(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	ids := seedArticles(b, db, 100)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = svc.GetByID(ctx, ids[i%100])
			i++
		}
	})
}

// BenchmarkConcurrentLikes measures parallel like operations.
func BenchmarkConcurrentLikes(b *testing.B) {
	db := setupBenchmarkDB(b)
	svc := newArticleService(b, db)
	ids := seedArticles(b, db, 100)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = svc.Like(ctx, ids[i%100])
			i++
		}
	})
}
