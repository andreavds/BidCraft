package auctions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository aísla el acceso a datos para poder probar el servicio sin PostgreSQL.
type Repository interface {
	Create(ctx context.Context, auction NewAuction) (Auction, error)
	FindByID(ctx context.Context, id int64) (Auction, error)
	List(ctx context.Context, filter ListFilter) ([]Auction, error)
}

// ListFilter son los criterios del catálogo. Status vacío significa "todas".
type ListFilter struct {
	Status Status
	Limit  int
	Offset int
}

// auctionColumns fija el orden de las columnas que espera scanAuction, con los
// nombres del autor y del ganador resueltos por JOIN: no se duplican en la tabla.
const auctionColumns = `a.id, a.created_by, author.full_name, winner.full_name,
	a.title, a.base_price, a.current_price, a.image_url,
	a.minimum_increment, a.start_at, a.end_at, a.status, a.winner_id, a.created_at, a.updated_at`

// auctionSource es la tabla con sus dos relaciones de usuario.
const auctionSource = `FROM auctions a
	JOIN users author ON author.id = a.created_by
	LEFT JOIN users winner ON winner.id = a.winner_id`

// querier admite tanto el pool como una transacción (pgx.Tx), de modo que el
// Milestone 4 pueda reutilizar estas lecturas dentro de la transacción de una puja.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, auction NewAuction) (Auction, error) {
	// start_at, end_at, current_price y status vienen ya calculados por el servicio:
	// el cliente no participa en ninguno de ellos.
	const insert = `
		INSERT INTO auctions (created_by, title, base_price, current_price, image_url,
			minimum_increment, start_at, end_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	var id int64
	err := r.pool.QueryRow(ctx, insert,
		auction.CreatedBy, auction.Title, auction.BasePrice, auction.CurrentPrice, auction.ImageURL,
		auction.MinimumIncrement, auction.StartAt, auction.EndAt, string(auction.Status)).Scan(&id)
	if err != nil {
		return Auction{}, fmt.Errorf("insert auction: %w", err)
	}

	return findByID(ctx, r.pool, id)
}

func (r *PostgresRepository) FindByID(ctx context.Context, id int64) (Auction, error) {
	return findByID(ctx, r.pool, id)
}

func (r *PostgresRepository) List(ctx context.Context, filter ListFilter) ([]Auction, error) {
	// El filtro se resuelve en SQL con un único parámetro opcional: cuando status
	// viene vacío la condición se cumple para todas las filas.
	query := `
		SELECT ` + auctionColumns + `
		` + auctionSource + `
		WHERE ($1 = '' OR a.status = $1)
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, string(filter.Status), filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("list auctions: %w", err)
	}
	defer rows.Close()

	auctions := make([]Auction, 0)
	for rows.Next() {
		auction, err := scanAuction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan auction: %w", err)
		}
		auctions = append(auctions, auction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read auctions: %w", err)
	}

	return auctions, nil
}

// LockForUpdate lee la subasta bloqueando su fila con SELECT ... FOR UPDATE: el
// lock se mantiene hasta que la transacción hace commit o rollback, de modo que
// ninguna otra transacción puede leer-modificar-escribir esa subasta entre medias.
//
// Devuelve además now() de PostgreSQL, para que la comprobación de expiración use
// el reloj de la base de datos y no el del proceso Go. En PostgreSQL now() es el
// instante de inicio de la transacción, así que es estable dentro del bloque.
//
// El paquete auctions es el dueño del SQL de su tabla; el motor de pujas compone
// esta lectura dentro de su propia transacción.
func LockForUpdate(ctx context.Context, tx pgx.Tx, id int64) (Auction, time.Time, error) {
	// Sin JOIN: bloquear la fila de la subasta no debe bloquear también las de
	// los usuarios. Los nombres no hacen falta para validar una puja.
	const query = `
		SELECT id, created_by, title, base_price, current_price, image_url,
			minimum_increment, start_at, end_at, status, winner_id, created_at, updated_at, now()
		FROM auctions WHERE id = $1 FOR UPDATE`

	var auction Auction
	var status string
	var dbNow time.Time

	err := tx.QueryRow(ctx, query, id).Scan(
		&auction.ID, &auction.CreatedBy, &auction.Title, &auction.BasePrice, &auction.CurrentPrice,
		&auction.ImageURL, &auction.MinimumIncrement, &auction.StartAt, &auction.EndAt, &status,
		&auction.WinnerID, &auction.CreatedAt, &auction.UpdatedAt, &dbNow,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Auction{}, time.Time{}, ErrNotFound
		}
		return Auction{}, time.Time{}, fmt.Errorf("lock auction: %w", err)
	}

	auction.Status = Status(status)

	return auction, dbNow, nil
}

// SetCurrentPrice actualiza el precio vigente dentro de la transacción que ya
// tiene bloqueada la fila, y devuelve el updated_at resultante.
func SetCurrentPrice(ctx context.Context, tx pgx.Tx, id int64, currentPrice int64) (time.Time, error) {
	const query = `
		UPDATE auctions
		SET current_price = $1, updated_at = now()
		WHERE id = $2
		RETURNING updated_at`

	var updatedAt time.Time
	if err := tx.QueryRow(ctx, query, currentPrice, id).Scan(&updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, ErrNotFound
		}
		return time.Time{}, fmt.Errorf("update current price: %w", err)
	}

	return updatedAt, nil
}

// ActiveAuction es una subasta activa con su plazo, para programar su cierre.
type ActiveAuction struct {
	ID    int64
	EndAt time.Time
}

// ListActive devuelve las subastas que siguen ACTIVE. La usa el scheduler al
// arrancar para reprogramar los timers, que viven en memoria y se pierden al
// reiniciar el proceso.
func (r *PostgresRepository) ListActive(ctx context.Context) ([]ActiveAuction, error) {
	const query = `SELECT id, end_at FROM auctions WHERE status = 'ACTIVE' ORDER BY end_at`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active auctions: %w", err)
	}
	defer rows.Close()

	active := make([]ActiveAuction, 0)
	for rows.Next() {
		var auction ActiveAuction
		if err := rows.Scan(&auction.ID, &auction.EndAt); err != nil {
			return nil, fmt.Errorf("scan active auction: %w", err)
		}
		active = append(active, auction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read active auctions: %w", err)
	}

	return active, nil
}

// findByID toma un querier para que la misma lectura sirva sobre el pool ahora y
// dentro de una transacción cuando exista el motor de pujas.
func findByID(ctx context.Context, q querier, id int64) (Auction, error) {
	query := `SELECT ` + auctionColumns + ` ` + auctionSource + ` WHERE a.id = $1`

	auction, err := scanAuction(q.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Auction{}, ErrNotFound
		}
		return Auction{}, fmt.Errorf("query auction: %w", err)
	}

	return auction, nil
}

// scanRow es lo común entre pgx.Row y pgx.Rows.
type scanRow interface {
	Scan(dest ...any) error
}

func scanAuction(row scanRow) (Auction, error) {
	var auction Auction
	var status string

	err := row.Scan(
		&auction.ID, &auction.CreatedBy, &auction.CreatedByName, &auction.WinnerName,
		&auction.Title, &auction.BasePrice, &auction.CurrentPrice, &auction.ImageURL,
		&auction.MinimumIncrement, &auction.StartAt, &auction.EndAt, &status, &auction.WinnerID,
		&auction.CreatedAt, &auction.UpdatedAt,
	)
	if err != nil {
		return Auction{}, err
	}

	auction.Status = Status(status)

	return auction, nil
}
