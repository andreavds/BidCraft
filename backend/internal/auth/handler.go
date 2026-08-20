package auth

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"bidcraft/internal/httpx"
)

// Handler traduce HTTP a llamadas al servicio y errores de dominio a códigos HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type registerRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// userResponse es la vista pública del usuario: se construye campo a campo,
// de modo que password_hash no puede acabar en una respuesta.
type userResponse struct {
	ID        int64     `json:"id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type authResponse struct {
	User  userResponse `json:"user"`
	Token string       `json:"token"`
}

func newUserResponse(user User) userResponse {
	return userResponse{ID: user.ID, FullName: user.FullName, Email: user.Email, CreatedAt: user.CreatedAt}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "Request body must be valid JSON")
		return
	}

	user, token, err := h.service.Register(r.Context(), req.FullName, req.Email, req.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, authResponse{User: newUserResponse(user), Token: token})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCredentials(w, r)
	if !ok {
		return
	}

	user, token, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, authResponse{User: newUserResponse(user), Token: token})
}

// Me responde con el usuario del token. El id llega por el contexto que puebla
// el middleware; nunca se lee del body ni de la query.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		// Solo ocurriría si la ruta se montara sin el middleware.
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	user, err := h.service.CurrentUser(r.Context(), userID)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, newUserResponse(user))
}

func decodeCredentials(w http.ResponseWriter, r *http.Request) (credentialsRequest, bool) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "Request body must be valid JSON")
		return credentialsRequest{}, false
	}
	return req, true
}

// writeAuthError es el único punto que decide el status HTTP, para que las
// respuestas sean consistentes entre endpoints.
func writeAuthError(w http.ResponseWriter, err error) {
	var validationErr ValidationError
	switch {
	case errors.As(err, &validationErr):
		httpx.Error(w, http.StatusBadRequest, "validation_error", validationErr.Message)
	case errors.Is(err, ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "email_taken", "Email is already registered")
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
	case errors.Is(err, ErrUserNotFound):
		// El token es válido pero el usuario ya no existe.
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
	default:
		// El detalle queda en el log del servidor; el cliente no ve errores SQL.
		log.Printf("auth: unexpected error: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
	}
}
