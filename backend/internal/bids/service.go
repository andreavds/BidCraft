package bids

import (
	"context"
	"errors"
	"log"

	"bidcraft/internal/auctions"
	"bidcraft/internal/websocket"
)

// Límites de paginación del historial, iguales a los del catálogo.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// Service orquesta la sincronización y la transacción. No conoce HTTP ni SQL.
type Service struct {
	repo  Repository
	locks *auctions.Locks
	hub   *websocket.Hub
}

// NewService admite hub nil: en ese caso no se emiten eventos, algo útil en tests.
func NewService(repo Repository, locks *auctions.Locks, hub *websocket.Hub) *Service {
	return &Service{repo: repo, locks: locks, hub: hub}
}

// Place registra una puja.
//
// El user_id llega del JWT, nunca del cuerpo del request.
//
// La sección crítica cubre toda la operación: mientras se mantiene el mutex de
// esta subasta, ninguna otra goroutine del proceso puede leer su precio,
// validar y escribirlo. Dentro, la transacción vuelve a serializar contra la
// base de datos con SELECT ... FOR UPDATE, que es la garantía real.
func (s *Service) Place(ctx context.Context, auctionID, userID, amount int64) (Bid, auctions.Auction, error) {
	if auctionID <= 0 {
		return Bid{}, auctions.Auction{}, ErrAuctionNotFound
	}
	if amount <= 0 {
		return Bid{}, auctions.Auction{}, ValidationError{Message: "amount must be greater than zero, in cents"}
	}

	lock := s.locks.Get(auctionID)
	lock.Lock()
	defer lock.Unlock()

	result, err := s.repo.Place(ctx, auctionID, userID, amount)
	if err != nil {
		logRejected(auctionID, userID, amount, err)
		return Bid{}, auctions.Auction{}, err
	}

	log.Printf("bid accepted: bid=%d auction=%d user=%d amount=%d current_price=%d",
		result.Bid.ID, auctionID, userID, amount, result.Auction.CurrentPrice)

	// La transacción ya hizo commit: solo ahora se notifica. Una puja rechazada
	// nunca llega hasta aquí, así que no puede generar eventos.
	s.notify(result)

	return result.Bid, result.Auction, nil
}

// notify emite bid_placed a toda la sala y outbid al postor superado.
func (s *Service) notify(result PlaceResult) {
	if s.hub == nil {
		return
	}

	bid := result.Bid
	s.hub.Broadcast(bid.AuctionID, websocket.Event{
		Type: "bid_placed",
		Data: map[string]any{
			"auction_id":    bid.AuctionID,
			"bid_id":        bid.ID,
			"user_id":       bid.UserID,
			"user_name":     bid.UserName,
			"amount":        bid.Amount,
			"current_price": result.Auction.CurrentPrice,
			"minimum_bid":   result.Auction.MinimumBid(),
			"created_at":    bid.CreatedAt,
		},
	})

	// Solo hay alguien a quien avisar si existía una puja anterior de otro usuario:
	// subir la propia puja no es quedarse superado.
	if result.PreviousBidderID == nil || *result.PreviousBidderID == bid.UserID {
		return
	}

	s.hub.SendToUser(bid.AuctionID, *result.PreviousBidderID, websocket.Event{
		Type: "outbid",
		Data: map[string]any{
			"auction_id":         bid.AuctionID,
			"previous_bidder_id": *result.PreviousBidderID,
			"new_amount":         bid.Amount,
		},
	})
}

// ListByAuction devuelve el historial de pujas, de la más reciente a la más antigua.
func (s *Service) ListByAuction(ctx context.Context, auctionID int64, limit, offset int) ([]Bid, error) {
	if auctionID <= 0 {
		return nil, ErrAuctionNotFound
	}

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Distingue "la subasta no existe" (404) de "existe y no tiene pujas" ([]).
	exists, err := s.repo.AuctionExists(ctx, auctionID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrAuctionNotFound
	}

	return s.repo.ListByAuction(ctx, auctionID, limit, offset)
}

// logRejected deja constancia de los rechazos esperables sin ensuciar el log con
// los errores inesperados, que ya registra el handler.
func logRejected(auctionID, userID, amount int64, err error) {
	var tooLow TooLowError
	switch {
	case errors.As(err, &tooLow):
		log.Printf("bid rejected: auction=%d user=%d amount=%d reason=bid_too_low minimum=%d",
			auctionID, userID, amount, tooLow.Minimum)
	case errors.Is(err, ErrAuctionClosed):
		log.Printf("bid rejected: auction=%d user=%d amount=%d reason=auction_closed", auctionID, userID, amount)
	case errors.Is(err, ErrOwnAuction):
		log.Printf("bid rejected: auction=%d user=%d amount=%d reason=own_auction", auctionID, userID, amount)
	case errors.Is(err, ErrAuctionNotFound):
		log.Printf("bid rejected: auction=%d user=%d amount=%d reason=auction_not_found", auctionID, userID, amount)
	}
}
