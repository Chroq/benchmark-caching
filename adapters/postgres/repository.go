package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Chroq/benchmark-caching/domain"
	"github.com/Chroq/benchmark-caching/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// 2. OPTIMIZED REPOSITORY (16-byte ULID Key, Protobuf Value, Partitioned)
// ============================================================================

type OptimizedRepository struct {
	pool *pgxpool.Pool
}

// NewOptimizedRepository creates a new optimized PostgreSQL repository.
func NewOptimizedRepository(pool *pgxpool.Pool) ports.OptimizedUserRepository {
	return &OptimizedRepository{
		pool: pool,
	}
}

// Get retrieves a UserData by its 16-byte ULID by querying the target physical partition directly (static partition pruning).
func (r *OptimizedRepository) Get(ctx context.Context, id [16]byte, dest *domain.UserData) (bool, error) {
	var value []byte

	// Static Partition Pruning: all queried keys are seeded keys, which expire at BootstrapTime + 2 hours
	targetTime := domain.BootstrapTime.Add(2 * time.Hour)
	partitionName := "cache_opt_partition_" + targetTime.Format("2006_01_02_15")

	query := "SELECT value FROM " + partitionName + " WHERE key = $1 AND expires_at > $2 LIMIT 1"

	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := r.pool.QueryRow(dbCtx,
		query,
		id,
		domain.CurrentTimeUTC.Load().(time.Time),
	).Scan(&value)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // Cache miss or expired
		}
		return false, fmt.Errorf("optimized get db error: %w", err)
	}

	// High-performance manual Protobuf decoding directly into recycled object
	if err := dest.UnmarshalProtobuf(value); err != nil {
		return false, fmt.Errorf("optimized protobuf decode error: %w", err)
	}

	return true, nil
}

// Set stores a UserData serialized to Protobuf with a ULID key, writing directly to the target partition (static partition pruning).
func (r *OptimizedRepository) Set(ctx context.Context, user *domain.UserData, ttl time.Duration) error {
	expiresAt := domain.CurrentTimeUTC.Load().(time.Time).Add(ttl)

	// Static Partition Pruning: determine partition based on expiresAt
	partitionName := "cache_opt_partition_" + expiresAt.Format("2006_01_02_15")

	// High-performance manual Protobuf encoding
	value, err := user.MarshalProtobuf()
	if err != nil {
		return fmt.Errorf("optimized protobuf encode error: %w", err)
	}

	query := "INSERT INTO " + partitionName + " (key, value, expires_at) VALUES ($1, $2, $3) ON CONFLICT (key, expires_at) DO UPDATE SET value = EXCLUDED.value"

	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err = r.pool.Exec(dbCtx,
		query,
		user.ID,
		value,
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("optimized set db error: %w", err)
	}

	return nil
}

type optimizedCopySource struct {
	users     []domain.UserData
	expiresAt time.Time
	index     int
	rowValues []any
	err       error
}

func (s *optimizedCopySource) Next() bool {
	if s.index >= len(s.users) {
		return false
	}
	u := &s.users[s.index]

	value, err := u.MarshalProtobuf()
	if err != nil {
		s.err = err
		return false
	}

	s.rowValues[0] = u.ID
	s.rowValues[1] = value
	s.rowValues[2] = s.expiresAt

	s.index++
	return true
}

func (s *optimizedCopySource) Values() ([]any, error) {
	return s.rowValues, nil
}

func (s *optimizedCopySource) Err() error {
	return s.err
}

// Seed populates the database using high-performance pgx CopyFrom directly to the partition table.
func (r *OptimizedRepository) Seed(ctx context.Context, users []domain.UserData, ttl time.Duration) error {
	targetTime := domain.BootstrapTime.Add(ttl)
	partitionName := "cache_opt_partition_" + targetTime.Format("2006_01_02_15")
	expiresAt := time.Now().Add(ttl).UTC()

	source := &optimizedCopySource{
		users:     users,
		expiresAt: expiresAt,
		rowValues: make([]any, 3),
	}

	_, err := r.pool.CopyFrom(
		ctx,
		pgx.Identifier{partitionName},
		[]string{"key", "value", "expires_at"},
		source,
	)
	if err != nil {
		return fmt.Errorf("failed to bulk copy optimized seed: %w", err)
	}
	return nil
}

// ============================================================================
// 3. STANDARD RELATIONAL REPOSITORY (Columns mapped directly to fields, flat)
// ============================================================================

type StandardRepository struct {
	pool *pgxpool.Pool
}

// NewStandardRepository creates a new standard relational PostgreSQL repository.
func NewStandardRepository(pool *pgxpool.Pool) ports.StandardUserRepository {
	return &StandardRepository{
		pool: pool,
	}
}

// Get retrieves a UserData by its 16-byte UUID from the relational table users_standard.
func (r *StandardRepository) Get(ctx context.Context, id [16]byte, dest *domain.UserData) (bool, error) {
	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Execute connection-level prepared statement "get_user_standard"
	err := r.pool.QueryRow(dbCtx,
		"get_user_standard",
		id,
		domain.CurrentTimeUTC.Load().(time.Time),
	).Scan(
		&dest.ID,
		&dest.FirstName,
		&dest.LastName,
		&dest.BirthDate,
		&dest.Active,
		&dest.CreatedAt,
		&dest.UpdatedAt,
		&dest.DeletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // Cache miss or expired
		}
		return false, fmt.Errorf("standard get db error: %w", err)
	}

	return true, nil
}

// Set stores a UserData by writing directly to standard SQL columns.
func (r *StandardRepository) Set(ctx context.Context, user *domain.UserData, ttl time.Duration) error {
	expiresAt := domain.CurrentTimeUTC.Load().(time.Time).Add(ttl)

	dbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Execute connection-level prepared statement "set_user_standard"
	_, err := r.pool.Exec(dbCtx,
		"set_user_standard",
		user.ID,
		user.FirstName,
		user.LastName,
		user.BirthDate,
		user.Active,
		user.CreatedAt,
		user.UpdatedAt,
		user.DeletedAt,
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("standard set db error: %w", err)
	}

	return nil
}

type standardCopySource struct {
	users     []domain.UserData
	expiresAt time.Time
	index     int
	rowValues []any
}

func (s *standardCopySource) Next() bool {
	if s.index >= len(s.users) {
		return false
	}
	u := &s.users[s.index]

	s.rowValues[0] = u.ID
	s.rowValues[1] = u.FirstName
	s.rowValues[2] = u.LastName
	s.rowValues[3] = u.BirthDate
	s.rowValues[4] = u.Active
	s.rowValues[5] = u.CreatedAt
	s.rowValues[6] = u.UpdatedAt
	s.rowValues[7] = u.DeletedAt
	s.rowValues[8] = s.expiresAt

	s.index++
	return true
}

func (s *standardCopySource) Values() ([]any, error) {
	return s.rowValues, nil
}

func (s *standardCopySource) Err() error {
	return nil
}

// Seed populates the standard relational table using high-performance pgx CopyFrom.
func (r *StandardRepository) Seed(ctx context.Context, users []domain.UserData, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl).UTC()

	source := &standardCopySource{
		users:     users,
		expiresAt: expiresAt,
		rowValues: make([]any, 9),
	}

	_, err := r.pool.CopyFrom(
		ctx,
		pgx.Identifier{"users_standard"},
		[]string{"id", "first_name", "last_name", "birth_date", "active", "created_at", "updated_at", "deleted_at", "expires_at"},
		source,
	)
	if err != nil {
		return fmt.Errorf("failed to bulk copy standard seed: %w", err)
	}
	return nil
}
