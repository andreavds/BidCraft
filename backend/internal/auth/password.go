package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// dummyHash se compara cuando el email no existe, para que un login fallido cueste
// lo mismo exista o no el usuario y no se pueda enumerar cuentas midiendo tiempos.
// Es el hash bcrypt de una cadena arbitraria; no corresponde a ninguna contraseña real.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// wastePasswordComparison mantiene constante el coste del login para emails inexistentes.
func wastePasswordComparison(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}
