package repository

import (
	"context"

	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"gorm.io/gorm"
)

type CommentRepository struct {
	repo *orm.Repository[domain.Comment]
	db   *gorm.DB
}

func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{repo: orm.NewRepository[domain.Comment](db), db: db}
}

func (r *CommentRepository) Create(ctx context.Context, comment *domain.Comment) error {
	return r.repo.Create(ctx, comment)
}

func (r *CommentRepository) FindByID(ctx context.Context, id uint) (*domain.Comment, error) {
	return r.repo.FindByID(ctx, id)
}

func (r *CommentRepository) FindByArticle(ctx context.Context, articleID uint, page orm.Page) ([]*domain.Comment, int64, error) {
	db := orm.FromCtx(ctx, r.db)
	var comments []*domain.Comment
	var total int64

	query := db.WithContext(ctx).Model(&domain.Comment{}).
		Where("article_id = ?", articleID).
		Preload("User")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Scopes(orm.Paginate(page)).Order("created_at ASC").Find(&comments).Error; err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

func (r *CommentRepository) IncrLikeCount(ctx context.Context, id uint) error {
	db := orm.FromCtx(ctx, r.db)
	return db.WithContext(ctx).Model(&domain.Comment{}).Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error
}

func (r *CommentRepository) Delete(ctx context.Context, id uint) error {
	return r.repo.Delete(ctx, id)
}
