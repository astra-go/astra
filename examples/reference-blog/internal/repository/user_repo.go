package repository

import (
	"context"
	"errors"

	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"gorm.io/gorm"
)

type UserRepository struct {
	repo *orm.Repository[domain.User]
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{repo: orm.NewRepository[domain.User](db)}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	return r.repo.Create(ctx, user)
}

func (r *UserRepository) FindByID(ctx context.Context, id uint) (*domain.User, error) {
	return r.repo.FindByID(ctx, id)
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	user, err := r.repo.First(ctx, "username = ?", username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := r.repo.First(ctx, "email = ?", email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	return r.repo.Updates(ctx, user.ID, user)
}
