package auth

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const (
	maxFullNameLength = 120
	minPasswordLength = 8
	// bcrypt silently ignores anything past 72 bytes, so reject longer inputs
	// instead of accepting a password that is only partially checked.
	maxPasswordLength = 72
)

// Errores de dominio. El handler los traduce a códigos HTTP.
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

// ValidationError agrupa los fallos de validación de entrada (HTTP 400).
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// User es el modelo de dominio. PasswordHash nunca se serializa: el tag `json:"-"`
// hace imposible filtrarlo aunque un handler devuelva el struct por error.
type User struct {
	ID           int64     `json:"id"`
	FullName     string    `json:"full_name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// normalizeEmail deja el email en la forma canónica que se guarda y se consulta,
// de modo que el UNIQUE de PostgreSQL distinga "A@x.com" de "a@x.com" como iguales.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(email string) error {
	if email == "" {
		return ValidationError{Message: "email is required"}
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return ValidationError{Message: "email is not a valid address"}
	}
	return nil
}

func validateFullName(fullName string) error {
	if fullName == "" {
		return ValidationError{Message: "full_name is required"}
	}
	if len(fullName) > maxFullNameLength {
		return ValidationError{Message: fmt.Sprintf("full_name must be at most %d characters", maxFullNameLength)}
	}
	return nil
}

func validatePassword(password string) error {
	if password == "" {
		return ValidationError{Message: "password is required"}
	}
	if len(password) < minPasswordLength {
		return ValidationError{Message: fmt.Sprintf("password must be at least %d characters", minPasswordLength)}
	}
	if len(password) > maxPasswordLength {
		return ValidationError{Message: fmt.Sprintf("password must be at most %d bytes", maxPasswordLength)}
	}
	return nil
}
