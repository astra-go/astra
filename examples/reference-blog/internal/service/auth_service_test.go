package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}))
	return db
}

func TestAuthService_Register_Success(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	result, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Token)
	assert.Equal(t, "testuser", result.User.Username)
	assert.Equal(t, "test@example.com", result.User.Email)
	assert.Equal(t, "reader", result.User.Role)
	assert.NotEmpty(t, result.User.PasswordHash)
}

func TestAuthService_Register_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	// Register first user
	_, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test1@example.com",
		Password: "password123",
	})
	require.NoError(t, err)

	// Try duplicate username
	result, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test2@example.com",
		Password: "password123",
	})

	assert.Nil(t, result)
	assert.Equal(t, "username already taken", err.Error())
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	_, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "user1",
		Email:    "same@example.com",
		Password: "password123",
	})
	require.NoError(t, err)

	result, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "user2",
		Email:    "same@example.com",
		Password: "password123",
	})

	assert.Nil(t, result)
	assert.Equal(t, "email already registered", err.Error())
}

func TestAuthService_Login_Success(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	// Register first
	_, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	})
	require.NoError(t, err)

	// Login
	result, err := svc.Login(context.Background(), service.LoginRequest{
		Username: "testuser",
		Password: "password123",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Token)
	assert.Equal(t, "testuser", result.User.Username)
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	result, err := svc.Login(context.Background(), service.LoginRequest{
		Username: "nonexistent",
		Password: "password123",
	})

	assert.Nil(t, result)
	assert.Equal(t, "invalid credentials", err.Error())
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	_, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	})
	require.NoError(t, err)

	result, err := svc.Login(context.Background(), service.LoginRequest{
		Username: "testuser",
		Password: "wrongpassword",
	})

	assert.Nil(t, result)
	assert.Equal(t, "invalid credentials", err.Error())
}

func TestAuthService_ValidateToken_Valid(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	regResult, err := svc.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	})
	require.NoError(t, err)

	claims, err := svc.ValidateToken(regResult.Token)

	assert.NoError(t, err)
	assert.Equal(t, "testuser", claims.Username)
	assert.Equal(t, "reader", claims.Role)
}

func TestAuthService_ValidateToken_Invalid(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, "test-secret", 24*time.Hour)

	claims, err := svc.ValidateToken("invalid-token")

	assert.Nil(t, claims)
	assert.Error(t, err)
}

func TestAuthService_ValidateToken_WrongSecret(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	svc1 := service.NewAuthService(userRepo, "secret-1", 24*time.Hour)
	svc2 := service.NewAuthService(userRepo, "secret-2", 24*time.Hour)

	regResult, _ := svc1.Register(context.Background(), service.RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	})

	claims, err := svc2.ValidateToken(regResult.Token)

	assert.Nil(t, claims)
	assert.Error(t, err)
}
