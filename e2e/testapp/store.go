package testapp

import (
	"errors"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrUsernameTaken  = errors.New("username already taken")
	ErrInvalidPassword = errors.New("invalid credentials")
)

// User is the in-memory user entity used by the e2e testapp.
type User struct {
	ID           string
	Username     string
	passwordHash string
}

// UserStore is a thread-safe in-memory user store.
type UserStore struct {
	mu    sync.RWMutex
	users map[string]*User // keyed by username
}

func NewUserStore() *UserStore {
	return &UserStore{users: make(map[string]*User)}
}

func (s *UserStore) Register(username, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return nil, ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return nil, err
	}

	u := &User{
		ID:           uuid.NewString(),
		Username:     username,
		passwordHash: string(hash),
	}
	s.users[username] = u
	return u, nil
}

func (s *UserStore) Login(username, password string) (*User, error) {
	s.mu.RLock()
	u, exists := s.users[username]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.passwordHash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}
	return u, nil
}

func (s *UserStore) GetByID(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}
