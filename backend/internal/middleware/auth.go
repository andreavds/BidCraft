// Package middleware contiene los middlewares HTTP propios del proyecto.
package middleware

import (
	"net/http"
	"strings"

	"bidcraft/internal/auth"
	"bidcraft/internal/httpx"
)

// RequireAuth rechaza con 401 cualquier request sin un Bearer token HS256 válido
// y no expirado, y deja el user_id del claim sub en el contexto.
func RequireAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				unauthorized(w)
				return
			}

			userID, err := auth.ParseToken(secret, token)
			if err != nil {
				// El motivo exacto (firma, expiración, algoritmo) no se revela al cliente.
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.ContextWithUserID(r.Context(), userID)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	return token, true
}

func unauthorized(w http.ResponseWriter) {
	httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
}
