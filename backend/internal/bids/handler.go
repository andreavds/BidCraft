package bids

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bidcraft/internal/auctions"
	"bidcraft/internal/auth"
	"bidcraft/internal/httpx"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// placeRequest es el único campo aceptado. Como en la creación de subastas, el
// decodificador rechaza cualquier otro: enviar user_id, auction_id o created_at
// devuelve 400 en vez de ignorarse en silencio.
type placeRequest struct {
	Amount int64 `json:"amount"`
}

type bidResponse struct {
	ID        int64     `json:"id"`
	AuctionID int64     `json:"auction_id"`
	UserID    int64     `json:"user_id"`
	UserName  string    `json:"user_name"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

// auctionStateResponse ahorra al cliente un GET adicional tras pujar y le da
// directamente el siguiente mínimo aceptable.
type auctionStateResponse struct {
	CurrentPrice int64           `json:"current_price"`
	MinimumBid   int64           `json:"minimum_bid"`
	Status       auctions.Status `json:"status"`
	EndAt        time.Time       `json:"end_at"`
}

type placeResponse struct {
	bidResponse
	Auction auctionStateResponse `json:"auction"`
}

type listResponse struct {
	Bids   []bidResponse `json:"bids"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

func newBidResponse(bid Bid) bidResponse {
	return bidResponse{
		ID:        bid.ID,
		AuctionID: bid.AuctionID,
		UserID:    bid.UserID,
		UserName:  bid.UserName,
		Amount:    bid.Amount,
		CreatedAt: bid.CreatedAt,
	}
}

// Place requiere autenticación.
func (h *Handler) Place(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		// Solo ocurriría si la ruta se montara sin el middleware.
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	auctionID, ok := parseAuctionID(w, r)
	if !ok {
		return
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req placeRequest
	if err := decoder.Decode(&req); err != nil {
		message := "Request body must be valid JSON"
		if strings.Contains(err.Error(), "unknown field") {
			message = "Unknown field in request body: only amount is accepted; the bidder comes from the JWT"
		}
		httpx.Error(w, http.StatusBadRequest, "validation_error", message)
		return
	}

	bid, auction, err := h.service.Place(r.Context(), auctionID, userID, req.Amount)
	if err != nil {
		writeError(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, placeResponse{
		bidResponse: newBidResponse(bid),
		Auction: auctionStateResponse{
			CurrentPrice: auction.CurrentPrice,
			MinimumBid:   auction.MinimumBid(),
			Status:       auction.Status,
			EndAt:        auction.EndAt,
		},
	})
}

// List es público.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	auctionID, ok := parseAuctionID(w, r)
	if !ok {
		return
	}

	limit, err := parseOptionalInt(r.URL.Query().Get("limit"), defaultLimit)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "limit must be a positive integer")
		return
	}

	offset, err := parseOptionalInt(r.URL.Query().Get("offset"), 0)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "offset must be a positive integer")
		return
	}

	bids, err := h.service.ListByAuction(r.Context(), auctionID, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	items := make([]bidResponse, 0, len(bids))
	for _, bid := range bids {
		items = append(items, newBidResponse(bid))
	}

	httpx.JSON(w, http.StatusOK, listResponse{Bids: items, Limit: limit, Offset: offset})
}

func parseAuctionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	auctionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "Auction id must be a positive integer")
		return 0, false
	}

	return auctionID, true
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
	var tooLow TooLowError

	switch {
	case errors.As(err, &validationErr):
		httpx.Error(w, http.StatusBadRequest, "validation_error", validationErr.Message)
	case errors.Is(err, ErrAuctionNotFound):
		httpx.Error(w, http.StatusNotFound, "auction_not_found", "Auction not found")
	case errors.Is(err, ErrAuctionClosed):
		httpx.Error(w, http.StatusConflict, "auction_closed", "Auction is no longer accepting bids")
	case errors.Is(err, ErrOwnAuction):
		httpx.Error(w, http.StatusForbidden, "own_auction", "You cannot bid on an auction you published")
	case errors.As(err, &tooLow):
		httpx.Error(w, http.StatusConflict, "bid_too_low",
			fmt.Sprintf("Minimum accepted bid is %s", FormatAmount(tooLow.Minimum)))
	default:
		// El detalle queda en el log del servidor; el cliente no ve errores SQL.
		log.Printf("bids: unexpected error: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
	}
}
