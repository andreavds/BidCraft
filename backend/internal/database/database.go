package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	connectAttempts = 10
	connectDelay    = time.Second
)

// Connect opens the pool and retries the first ping: outside Docker Compose
// nothing guarantees PostgreSQL is already accepting connections.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	for attempt := 1; ; attempt++ {
		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}

		if attempt == connectAttempts || ctx.Err() != nil {
			pool.Close()
			return nil, fmt.Errorf("ping database after %d attempts: %w", attempt, err)
		}

		log.Printf("postgres not ready (attempt %d/%d), retrying in %s", attempt, connectAttempts, connectDelay)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(connectDelay):
		}
	}
}
