package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenTTL es la vida de un access token. No hay refresh tokens: al expirar,
// el cliente vuelve a hacer login.
const TokenTTL = 24 * time.Hour

var ErrInvalidToken = errors.New("invalid token")

// GenerateToken firma un HS256 con los claims mínimos: sub, iat y exp.
func GenerateToken(secret string, userID int64, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

// ParseToken valida firma y expiración y devuelve el user_id del claim sub.
// El algoritmo se restringe explícitamente a HS256: sin esa restricción un token
// con "alg": "none" u otro algoritmo podría aceptarse.
func ParseToken(secret, tokenString string) (int64, error) {
	claims := &jwt.RegisteredClaims{}

	_, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("%w: subject is not a valid user id", ErrInvalidToken)
	}

	return userID, nil
}
