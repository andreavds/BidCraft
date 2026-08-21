package auctions

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"bidcraft/internal/websocket"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Closer cierra una subasta cuando vence su plazo.
type Closer struct {
	pool  *pgxpool.Pool
	locks *Locks
	hub   *websocket.Hub
}

// NewCloser admite hub nil: en ese caso no se emite el evento de cierre.
func NewCloser(pool *pgxpool.Pool, locks *Locks, hub *websocket.Hub) *Closer {
	return &Closer{pool: pool, locks: locks, hub: hub}
}

// Close pasa la subasta a FINISHED y persiste el ganador, todo en una
// transacción:
//
//	BEGIN
//	  SELECT ... FOR UPDATE      (bloquea la subasta)
//	  comprobar ACTIVE y vencida
//	  buscar la puja más alta    (el ganador)
//	  UPDATE status/winner_id/current_price
//	COMMIT
//
// Toma el mismo mutex por subasta que el motor de pujas, así que una puja y el
// cierre nunca modifican la misma subasta a la vez: si la puja llega primero,
// el cierre la ve y puede ser la ganadora; si el cierre llega primero, la puja
// encuentra FINISHED y se rechaza.
//
// Devuelve:
//   - closed: true si esta llamada cerró la subasta.
//   - retryAt: instante en el que volver a intentarlo, cuando el timer se
//     adelantó y la subasta todavía no había vencido. Cero si no hay que
//     reintentar (no existe o ya estaba cerrada).
func (c *Closer) Close(ctx context.Context, auctionID int64) (closed bool, retryAt time.Time, err error) {
	lock := c.locks.Get(auctionID)
	lock.Lock()
	defer lock.Unlock()

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	auction, dbNow, err := LockForUpdate(ctx, tx, auctionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}

	if auction.Status != StatusActive {
		return false, time.Time{}, nil
	}

	// El reloj que manda es el de PostgreSQL, el mismo con el que se validan las
	// pujas. El timer usa el reloj del proceso Go y los dos derivan unos pocos
	// milisegundos, así que puede adelantarse: en ese caso no se cierra nada y se
	// pide reintentar cuando de verdad haya vencido.
	if dbNow.Before(auction.EndAt) {
		return false, auction.EndAt, nil
	}

	var winnerID *int64
	var winnerName *string
	finalPrice := auction.BasePrice

	var bidderID, amount int64
	var bidderName string
	err = tx.QueryRow(ctx,
		`SELECT b.user_id, u.full_name, b.amount
		 FROM bids b JOIN users u ON u.id = b.user_id
		 WHERE b.auction_id = $1 ORDER BY b.amount DESC LIMIT 1`,
		auctionID).Scan(&bidderID, &bidderName, &amount)
	switch {
	case err == nil:
		winnerID = &bidderID
		winnerName = &bidderName
		finalPrice = amount
	case errors.Is(err, pgx.ErrNoRows):
		// Subasta sin pujas: se cierra igualmente, sin ganador.
	default:
		return false, time.Time{}, fmt.Errorf("find winning bid: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE auctions
		 SET status = $1, winner_id = $2, current_price = $3, updated_at = now()
		 WHERE id = $4`,
		string(StatusFinished), winnerID, finalPrice, auctionID)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("finish auction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, time.Time{}, fmt.Errorf("commit auction close: %w", err)
	}

	if winnerID == nil {
		log.Printf("auction closed: id=%d winner=none final_price=%d", auctionID, finalPrice)
	} else {
		log.Printf("auction closed: id=%d winner=%d final_price=%d", auctionID, *winnerID, finalPrice)
	}

	if c.hub != nil {
		event := websocket.Event{
			Type: "auction_finished",
			Data: map[string]any{
				"auction_id":  auctionID,
				"status":      string(StatusFinished),
				"winner_id":   winnerID,
				"winner_name": winnerName,
				"final_price": finalPrice,
			},
		}

		// A la sala, para la página de la subasta; y al catálogo, para que la
		// tarjeta pase a FINISHED sin esperar a que alguien recargue la lista.
		c.hub.Broadcast(auctionID, event)
		c.hub.BroadcastCatalog(event)
	}

	return true, time.Time{}, nil
}
