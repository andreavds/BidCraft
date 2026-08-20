package bids

import (
	"context"
	"errors"
	"fmt"

	"bidcraft/internal/auctions"

	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PlaceResult es el desenlace de una puja aceptada, ya confirmada en la base de
// datos. PreviousBidderID es el postor al que esta puja acaba de superar, y es
// nil cuando la subasta no tenía pujas: lo necesita el evento outbid.
type PlaceResult struct {
	Bid              Bid
	Auction          auctions.Auction
	PreviousBidderID *int64
}

// Repository aísla el acceso a datos para poder probar el servicio sin PostgreSQL.
type Repository interface {
	// Place ejecuta la operación atómica completa y devuelve la puja aceptada
	// junto con el estado resultante de la subasta.
	Place(ctx context.Context, auctionID, userID, amount int64) (PlaceResult, error)
	ListByAuction(ctx context.Context, auctionID int64, limit, offset int) ([]Bid, error)
	AuctionExists(ctx context.Context, auctionID int64) (bool, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Place es el núcleo del motor de pujas.
//
//	BEGIN
//	  SELECT ... FOR UPDATE   (bloquea la subasta)
//	  validar estado, expiración e importe
//	  INSERT bid
//	  UPDATE auctions.current_price / updated_at
//	COMMIT
//
// Cualquier fallo o rechazo sale por rollback, así que nunca puede quedar una
// puja sin su precio ni un precio sin su puja. El aislamiento por defecto
// (READ COMMITTED) basta: con FOR UPDATE, la transacción que espera re-lee la
// fila ya actualizada al obtener el lock, sin necesidad de reintentos.
func (r *PostgresRepository) Place(ctx context.Context, auctionID, userID, amount int64) (PlaceResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PlaceResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	auction, dbNow, err := auctions.LockForUpdate(ctx, tx, auctionID)
	if err != nil {
		if errors.Is(err, auctions.ErrNotFound) {
			return PlaceResult{}, ErrAuctionNotFound
		}
		return PlaceResult{}, err
	}

	if err := Validate(auction, userID, amount, dbNow); err != nil {
		return PlaceResult{}, err
	}

	// Quién iba ganando antes de esta puja: es el destinatario del evento outbid.
	// Se lee dentro de la transacción, con la subasta ya bloqueada.
	var previousBidderID *int64
	var previous int64
	switch err := tx.QueryRow(ctx,
		`SELECT user_id FROM bids WHERE auction_id = $1 ORDER BY id DESC LIMIT 1`,
		auctionID).Scan(&previous); {
	case err == nil:
		previousBidderID = &previous
	case errors.Is(err, pgx.ErrNoRows):
		// Primera puja de la subasta: no hay a quién avisar.
	default:
		return PlaceResult{}, fmt.Errorf("query previous bidder: %w", err)
	}

	const insertBid = `
		INSERT INTO bids (auction_id, user_id, amount)
		VALUES ($1, $2, $3)
		RETURNING id, auction_id, user_id, amount, created_at,
			(SELECT full_name FROM users WHERE id = $2)`

	var bid Bid
	err = tx.QueryRow(ctx, insertBid, auctionID, userID, amount).
		Scan(&bid.ID, &bid.AuctionID, &bid.UserID, &bid.Amount, &bid.CreatedAt, &bid.UserName)
	if err != nil {
		return PlaceResult{}, fmt.Errorf("insert bid: %w", err)
	}

	updatedAt, err := auctions.SetCurrentPrice(ctx, tx, auctionID, amount)
	if err != nil {
		return PlaceResult{}, fmt.Errorf("update auction price: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return PlaceResult{}, fmt.Errorf("commit bid: %w", err)
	}

	// A partir de aquí la puja está persistida: el estado devuelto refleja el
	// commit, y el servicio ya puede notificar por WebSocket.
	auction.CurrentPrice = amount
	auction.UpdatedAt = updatedAt

	return PlaceResult{Bid: bid, Auction: auction, PreviousBidderID: previousBidderID}, nil
}

func (r *PostgresRepository) ListByAuction(ctx context.Context, auctionID int64, limit, offset int) ([]Bid, error) {
	// El orden autoritativo es id DESC: los INSERT de una subasta están
	// serializados por el row lock, así que el orden de los id es el orden real
	// de aceptación. created_at es informativo.
	const query = `
		SELECT b.id, b.auction_id, b.user_id, u.full_name, b.amount, b.created_at
		FROM bids b
		JOIN users u ON u.id = b.user_id
		WHERE b.auction_id = $1
		ORDER BY b.id DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, auctionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list bids: %w", err)
	}
	defer rows.Close()

	bids := make([]Bid, 0)
	for rows.Next() {
		var bid Bid
		if err := rows.Scan(&bid.ID, &bid.AuctionID, &bid.UserID, &bid.UserName, &bid.Amount, &bid.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan bid: %w", err)
		}
		bids = append(bids, bid)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read bids: %w", err)
	}

	return bids, nil
}

func (r *PostgresRepository) AuctionExists(ctx context.Context, auctionID int64) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM auctions WHERE id = $1)", auctionID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check auction: %w", err)
	}

	return exists, nil
}

// compile-time: PostgresRepository implementa Repository.
var _ Repository = (*PostgresRepository)(nil)
