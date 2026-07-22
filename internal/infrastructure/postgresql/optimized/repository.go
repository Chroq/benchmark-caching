package optimized

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/serializer"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OptimizedRepository implements unlogged partitioned key-value cache storage with Protobuf values.
type OptimizedRepository struct {
	pool *pgxpool.Pool
}

// CleanOptimizedTable truncates the unlogged cache_optimized table during setup.
func CleanOptimizedTable(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("Truncating cache_optimized table...")
	_, err := pool.Exec(ctx, "TRUNCATE TABLE cache_optimized")
	if err != nil {
		return fmt.Errorf("failed to truncate cache_optimized: %w", err)
	}
	return nil
}

// NewRepository creates a new optimized PostgreSQL repository.
func NewRepository(pool *pgxpool.Pool) output.OptimizedUserRepository {
	return &OptimizedRepository{
		pool: pool,
	}
}

// NewOptimizedRepository is an alias for NewRepository.
func NewOptimizedRepository(pool *pgxpool.Pool) output.OptimizedUserRepository {
	return NewRepository(pool)
}

// Close closes the underlying PostgreSQL connection pool.
func (r *OptimizedRepository) Close() error {
	if r.pool != nil {
		r.pool.Close()
	}
	return nil
}

// Get retrieves a UserData by its 16-byte UUID v7 by querying the unlogged cache_optimized table.
func (r *OptimizedRepository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	var value []byte

	err := r.pool.QueryRow(ctx,
		"get_user_optimized",
		id,
		time.Now().UTC(),
	).Scan(&value)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("optimized get db error: %w", err)
	}

	if err := serializer.UnmarshalProtobuf(value, dest); err != nil {
		return false, fmt.Errorf("optimized protobuf decode error: %w", err)
	}

	return true, nil
}

// Set stores a UserData serialized to Protobuf with a UUID v7 key into the cache_optimized table.
func (r *OptimizedRepository) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)

	value, err := serializer.MarshalProtobuf(user)
	if err != nil {
		return fmt.Errorf("optimized protobuf encode error: %w", err)
	}

	_, err = r.pool.Exec(ctx, "set_user_optimized", user.ID, value, expiresAt)
	if err != nil {
		return fmt.Errorf("optimized set db error: %w", err)
	}

	return nil
}
