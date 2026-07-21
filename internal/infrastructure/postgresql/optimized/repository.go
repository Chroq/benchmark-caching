package optimized

import (
	"context"
	"errors"
	"fmt"
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

// Get retrieves a UserData by its 16-byte ULID by querying the unlogged cache_optimized table.
func (r *OptimizedRepository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	var value []byte

	err := r.pool.QueryRow(ctx,
		"SELECT value FROM cache_optimized WHERE key = $1",
		id,
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

// Set stores a UserData serialized to Protobuf with a ULID key into the cache_optimized table.
func (r *OptimizedRepository) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)

	value, err := serializer.MarshalProtobuf(user)
	if err != nil {
		return fmt.Errorf("optimized protobuf encode error: %w", err)
	}

	query := "INSERT INTO cache_optimized (key, value, expires_at) VALUES ($1, $2, $3) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at"

	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err = r.pool.Exec(dbCtx, query, user.ID, value, expiresAt)
	if err != nil {
		return fmt.Errorf("optimized set db error: %w", err)
	}

	return nil
}
