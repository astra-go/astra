package repository_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}))
	return db
}

func TestUserRepository_Create(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	user := &domain.User{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         "reader",
	}

	err := repo.Create(context.Background(), user)
	assert.NoError(t, err)
	assert.NotZero(t, user.ID)
}

func TestUserRepository_FindByID(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	user := &domain.User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash", Role: "reader"}
	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByID(context.Background(), user.ID)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", found.Username)
}

func TestUserRepository_FindByID_NotFound(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	found, err := repo.FindByID(context.Background(), 999)
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestUserRepository_FindByUsername(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	user := &domain.User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash", Role: "reader"}
	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByUsername(context.Background(), "testuser")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, user.ID, found.ID)
}

func TestUserRepository_FindByUsername_NotFound(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	found, err := repo.FindByUsername(context.Background(), "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestUserRepository_FindByEmail(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	user := &domain.User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash", Role: "reader"}
	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByEmail(context.Background(), "test@example.com")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, user.ID, found.ID)
}

func TestUserRepository_FindByEmail_NotFound(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	found, err := repo.FindByEmail(context.Background(), "none@example.com")
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestUserRepository_Update(t *testing.T) {
	db := setupUserTestDB(t)
	repo := repository.NewUserRepository(db)

	user := &domain.User{Username: "testuser", Email: "test@example.com", PasswordHash: "hash", Role: "reader"}
	require.NoError(t, repo.Create(context.Background(), user))

	user.Role = "author"
	err := repo.Update(context.Background(), user)
	assert.NoError(t, err)

	found, _ := repo.FindByID(context.Background(), user.ID)
	assert.Equal(t, "author", found.Role)
}
