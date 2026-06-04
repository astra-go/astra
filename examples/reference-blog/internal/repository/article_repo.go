package repository

import (
	"context"

	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"gorm.io/gorm"
)

type ArticleRepository struct {
	repo *orm.Repository[domain.Article]
	db   *gorm.DB
}

func NewArticleRepository(db *gorm.DB) *ArticleRepository {
	return &ArticleRepository{repo: orm.NewRepository[domain.Article](db), db: db}
}

func (r *ArticleRepository) Create(ctx context.Context, article *domain.Article) error {
	return r.repo.Create(ctx, article)
}

func (r *ArticleRepository) FindByID(ctx context.Context, id uint) (*domain.Article, error) {
	db := orm.FromCtx(ctx, r.db)
	var article domain.Article
	if err := db.WithContext(ctx).Preload("Author").First(&article, id).Error; err != nil {
		return nil, err
	}
	return &article, nil
}

func (r *ArticleRepository) FindPublished(ctx context.Context, page orm.Page) ([]*domain.Article, int64, error) {
	db := orm.FromCtx(ctx, r.db)
	var articles []*domain.Article
	var total int64

	query := db.WithContext(ctx).Model(&domain.Article{}).
		Where("status = ?", domain.ArticleStatusPublished).
		Preload("Author")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Scopes(orm.Paginate(page)).Order("created_at DESC").Find(&articles).Error; err != nil {
		return nil, 0, err
	}
	return articles, total, nil
}

func (r *ArticleRepository) FindByAuthor(ctx context.Context, authorID uint, page orm.Page) ([]*domain.Article, int64, error) {
	db := orm.FromCtx(ctx, r.db)
	var articles []*domain.Article
	var total int64

	query := db.WithContext(ctx).Model(&domain.Article{}).Where("author_id = ?", authorID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Scopes(orm.Paginate(page)).Order("created_at DESC").Find(&articles).Error; err != nil {
		return nil, 0, err
	}
	return articles, total, nil
}

func (r *ArticleRepository) Update(ctx context.Context, article *domain.Article) error {
	return r.repo.Updates(ctx, article.ID, article)
}

func (r *ArticleRepository) UpdateStatus(ctx context.Context, id uint, status domain.ArticleStatus) error {
	db := orm.FromCtx(ctx, r.db)
	return db.WithContext(ctx).Model(&domain.Article{}).Where("id = ?", id).
		Update("status", status).Error
}

func (r *ArticleRepository) IncrViewCount(ctx context.Context, id uint) error {
	db := orm.FromCtx(ctx, r.db)
	return db.WithContext(ctx).Model(&domain.Article{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

func (r *ArticleRepository) IncrLikeCount(ctx context.Context, id uint) error {
	db := orm.FromCtx(ctx, r.db)
	return db.WithContext(ctx).Model(&domain.Article{}).Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error
}

func (r *ArticleRepository) Delete(ctx context.Context, id uint) error {
	return r.repo.Delete(ctx, id)
}
