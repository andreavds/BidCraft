package auctions

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bidcraft/internal/auth"
	"bidcraft/internal/httpx"

	"github.com/go-chi/chi/v5"
)

// Handler traduce HTTP a llamadas al servicio.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// createRequest son los únicos campos aceptados en el body. El decodificador
// rechaza cualquier otro, así que un cliente que intente enviar current_price,
// status, end_at o winner_id recibe un 400 en lugar de que se ignore en silencio.
type createRequest struct {
	Title            string  `json:"title"`
	BasePrice        int64   `json:"base_price"`
	ImageURL         *string `json:"image_url"`
	MinimumIncrement int64   `json:"minimum_increment"`
	DurationSeconds  int64   `json:"duration_seconds"`
}

type auctionResponse struct {
	ID               int64     `json:"id"`
	Title            string    `json:"title"`
	Artist           string    `json:"artist"`
	CreatedBy        int64     `json:"created_by"`
	BasePrice        int64     `json:"base_price"`
	CurrentPrice     int64     `json:"current_price"`
	ImageURL         *string   `json:"image_url"`
	MinimumIncrement int64     `json:"minimum_increment"`
	MinimumBid       int64     `json:"minimum_bid"`
	StartAt          time.Time `json:"start_at"`
	EndAt            time.Time `json:"end_at"`
	Status           Status    `json:"status"`
	WinnerID         *int64    `json:"winner_id"`
	WinnerName       *string   `json:"winner_name"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type listResponse struct {
	Auctions []auctionResponse `json:"auctions"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

func newAuctionResponse(auction Auction) auctionResponse {
	return auctionResponse{
		ID:               auction.ID,
		Title:            auction.Title,
		Artist:           auction.CreatedByName,
		CreatedBy:        auction.CreatedBy,
		BasePrice:        auction.BasePrice,
		CurrentPrice:     auction.CurrentPrice,
		ImageURL:         auction.ImageURL,
		MinimumIncrement: auction.MinimumIncrement,
		MinimumBid:       auction.MinimumBid(),
		StartAt:          auction.StartAt,
		EndAt:            auction.EndAt,
		Status:           auction.Status,
		WinnerID:         auction.WinnerID,
		WinnerName:       auction.WinnerName,
		CreatedAt:        auction.CreatedAt,
		UpdatedAt:        auction.UpdatedAt,
	}
}

// Create requiere autenticación: el middleware ya validó el JWT.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		// Solo ocurriría si la ruta se montara sin el middleware.
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req createRequest
	if err := decoder.Decode(&req); err != nil {
		message := "Request body must be valid JSON"
		if strings.Contains(err.Error(), "unknown field") {
			message = "Unknown field in request body: the server sets current_price, status, start_at, end_at and winner_id"
		}
		httpx.Error(w, http.StatusBadRequest, "validation_error", message)
		return
	}

	auction, err := h.service.Create(r.Context(), CreateInput{
		CreatedBy:        userID,
		Title:            req.Title,
		BasePrice:        req.BasePrice,
		ImageURL:         req.ImageURL,
		MinimumIncrement: req.MinimumIncrement,
		DurationSeconds:  req.DurationSeconds,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	log.Printf("auction created: id=%d by user=%d ends_at=%s", auction.ID, userID, auction.EndAt.Format(time.RFC3339))
	httpx.JSON(w, http.StatusCreated, newAuctionResponse(auction))
}

// Get es público.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "Auction id must be a positive integer")
		return
	}

	auction, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, newAuctionResponse(auction))
}

// List es público. Acepta ?status=ACTIVE|FINISHED y paginación simple.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	limit, err := parseOptionalInt(query.Get("limit"), defaultLimit)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "limit must be a positive integer")
		return
	}

	offset, err := parseOptionalInt(query.Get("offset"), 0)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "offset must be a positive integer")
		return
	}

	filter := ListFilter{
		Status: Status(strings.ToUpper(strings.TrimSpace(query.Get("status")))),
		Limit:  limit,
		Offset: offset,
	}

	auctions, err := h.service.List(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}

	items := make([]auctionResponse, 0, len(auctions))
	for _, auction := range auctions {
		items = append(items, newAuctionResponse(auction))
	}

	httpx.JSON(w, http.StatusOK, listResponse{Auctions: items, Limit: filter.Limit, Offset: filter.Offset})
}

func parseOptionalInt(raw string, fallback int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New("invalid integer")
	}

	return value, nil
}

func writeError(w http.ResponseWriter, err error) {
	var validationErr ValidationError
	switch {
	case errors.As(err, &validationErr):
		httpx.Error(w, http.StatusBadRequest, "validation_error", validationErr.Message)
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "auction_not_found", "Auction not found")
	default:
		// El detalle queda en el log del servidor; el cliente no ve errores SQL.
		log.Printf("auctions: unexpected error: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
	}
}
