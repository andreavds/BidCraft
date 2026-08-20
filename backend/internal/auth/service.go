package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Service contiene las reglas de autenticación. No conoce HTTP ni SQL.
type Service struct {
	repo   UserRepository
	secret string
	now    func() time.Time
}

func NewService(repo UserRepository, secret string) *Service {
	return &Service{repo: repo, secret: secret, now: time.Now}
}

// Register crea el usuario y devuelve un token para no obligar a un login inmediato.
func (s *Service) Register(ctx context.Context, fullName, email, password string) (User, string, error) {
	fullName = strings.TrimSpace(fullName)
	if err := validateFullName(fullName); err != nil {
		return User{}, "", err
	}

	email = normalizeEmail(email)
	if err := validateEmail(email); err != nil {
		return User{}, "", err
	}
	if err := validatePassword(password); err != nil {
		return User{}, "", err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return User{}, "", err
	}

	user, err := s.repo.Create(ctx, fullName, email, hash)
	if err != nil {
		return User{}, "", err
	}

	token, err := GenerateToken(s.secret, user.ID, s.now())
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// Login devuelve ErrInvalidCredentials tanto si el email no existe como si la
// contraseña no coincide: distinguirlos permitiría enumerar cuentas registradas.
func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return User{}, "", ValidationError{Message: "email and password are required"}
	}

	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Sin este trabajo equivalente, un email inexistente respondería
			// mucho más rápido que uno con contraseña incorrecta.
			wastePasswordComparison(password)
			return User{}, "", ErrInvalidCredentials
		}
		return User{}, "", err
	}

	if !verifyPassword(user.PasswordHash, password) {
		return User{}, "", ErrInvalidCredentials
	}

	token, err := GenerateToken(s.secret, user.ID, s.now())
	if err != nil {
		return User{}, "", err
	}

	return user, token, nil
}

// CurrentUser resuelve el usuario del id que el middleware extrajo del JWT.
func (s *Service) CurrentUser(ctx context.Context, userID int64) (User, error) {
	return s.repo.FindByID(ctx, userID)
}
