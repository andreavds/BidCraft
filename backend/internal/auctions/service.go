package auctions

import (
	"context"
	"time"

	"bidcraft/internal/websocket"
)

// Límites de paginación del catálogo.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// Service contiene las reglas de las subastas. No conoce HTTP ni SQL.
type Service struct {
	repo      Repository
	scheduler *Scheduler
	hub       *websocket.Hub
	now       func() time.Time
}

// NewService admite scheduler y hub nil: en ese caso no se programa el cierre
// automático ni se emiten eventos, algo útil en tests.
func NewService(repo Repository, scheduler *Scheduler, hub *websocket.Hub) *Service {
	return &Service{repo: repo, scheduler: scheduler, hub: hub, now: time.Now}
}

// Create valida la entrada del cliente, deriva el estado inicial y persiste.
// El cliente solo aporta title, base_price, image_url, minimum_increment y
// duration_seconds; el resto lo decide el servidor.
func (s *Service) Create(ctx context.Context, input CreateInput) (Auction, error) {
	newAuction, err := input.build(s.now())
	if err != nil {
		return Auction{}, err
	}

	auction, err := s.repo.Create(ctx, newAuction)
	if err != nil {
		return Auction{}, err
	}

	if s.scheduler != nil {
		s.scheduler.Schedule(auction.ID, auction.EndAt)
	}

	// Ya está persistida: solo ahora se avisa al catálogo, igual que una puja no
	// se notifica hasta estar confirmada. Se envía la misma representación que
	// devuelve GET /api/v1/auctions para que el cliente no tenga que reconstruirla.
	if s.hub != nil {
		s.hub.BroadcastCatalog(websocket.Event{
			Type: "auction_created",
			Data: newAuctionResponse(auction),
		})
	}

	return auction, nil
}

// Get devuelve una subasta por id, o ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (Auction, error) {
	if id <= 0 {
		return Auction{}, ErrNotFound
	}

	return s.repo.FindByID(ctx, id)
}

// List devuelve el catálogo. Un status vacío significa "todas"; cualquier otro
// valor distinto de ACTIVE o FINISHED es un error de validación.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Auction, error) {
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, ValidationError{Message: "status must be ACTIVE or FINISHED"}
	}

	if filter.Limit <= 0 {
		filter.Limit = defaultLimit
	}
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	return s.repo.List(ctx, filter)
}
