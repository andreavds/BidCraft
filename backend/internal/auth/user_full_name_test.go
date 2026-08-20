package auth

import (
	"context"
	"testing"
	"time"
)

type fullNameRepository struct {
	created User
}

func (r *fullNameRepository) Create(_ context.Context, fullName, email, passwordHash string) (User, error) {
	r.created = User{
		ID:           1,
		FullName:     fullName,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}
	return r.created, nil
}

func (r *fullNameRepository) FindByEmail(context.Context, string) (User, error) {
	return User{}, ErrUserNotFound
}

func (r *fullNameRepository) FindByID(context.Context, int64) (User, error) {
	return r.created, nil
}

func TestRegisterPersistsFullName(t *testing.T) {
	repository := &fullNameRepository{}
	service := NewService(repository, "test-secret")

	user, _, err := service.Register(context.Background(), "  John Smith  ", "john@example.com", "password123")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if repository.created.FullName != "John Smith" {
		t.Errorf("stored full name = %q, want %q", repository.created.FullName, "John Smith")
	}
	if user.FullName != "John Smith" {
		t.Errorf("returned full name = %q, want %q", user.FullName, "John Smith")
	}
}