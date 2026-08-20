package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository aísla el acceso a datos para que el servicio pueda probarse
// con una implementación en memoria, sin PostgreSQL.
type UserRepository interface {
	Create(ctx context.Context, fullName, email, passwordHash string) (User, error)
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id int64) (User, error)
}

// PostgresUserRepository es la única pieza del paquete que conoce SQL.
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

func (r *PostgresUserRepository) Create(ctx context.Context, fullName, email, passwordHash string) (User, error) {
	const query = `
		INSERT INTO users (full_name, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, full_name, email, password_hash, created_at`

	var user User
	err := r.pool.QueryRow(ctx, query, fullName, email, passwordHash).
		Scan(&user.ID, &user.FullName, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		// La unicidad la decide la constraint de PostgreSQL, no un SELECT previo:
		// así dos registros simultáneos con el mismo email no pueden colarse ambos.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}

	return user, nil
}

func (r *PostgresUserRepository) FindByEmail(ctx context.Context, email string) (User, error) {
	const query = `SELECT id, full_name, email, password_hash, created_at FROM users WHERE email = $1`
	return r.queryUser(ctx, query, email)
}

func (r *PostgresUserRepository) FindByID(ctx context.Context, id int64) (User, error) {
	const query = `SELECT id, full_name, email, password_hash, created_at FROM users WHERE id = $1`
	return r.queryUser(ctx, query, id)
}

func (r *PostgresUserRepository) queryUser(ctx context.Context, query string, arg any) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, query, arg).
		Scan(&user.ID, &user.FullName, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("query user: %w", err)
	}
	return user, nil
}
