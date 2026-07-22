package standard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
	googleuuid "github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StandardRepository implements standard relational storage (columns mapped directly to fields).
type StandardRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new standard relational PostgreSQL repository.
func NewRepository(pool *pgxpool.Pool) output.StandardUserRepository {
	return &StandardRepository{
		pool: pool,
	}
}

// NewStandardRepository is an alias for NewRepository.
func NewStandardRepository(pool *pgxpool.Pool) output.StandardUserRepository {
	return NewRepository(pool)
}

// Close closes the underlying PostgreSQL connection pool.
func (r *StandardRepository) Close() error {
	if r.pool != nil {
		r.pool.Close()
	}
	return nil
}

// Get retrieves a UserData by its 16-byte UUID from the relational table users_standard.
func (r *StandardRepository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	err := r.pool.QueryRow(ctx,
		"get_user_standard",
		id,
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
			return false, nil
		}
		return false, fmt.Errorf("standard get db error: %w", err)
	}

	return true, nil
}

// Set stores a UserData by writing directly to standard SQL columns.
func (r *StandardRepository) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	_, err := r.pool.Exec(ctx,
		"set_user_standard",
		user.ID,
		user.FirstName,
		user.LastName,
		user.BirthDate,
		user.Active,
		user.CreatedAt,
		user.UpdatedAt,
		user.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("standard set db error: %w", err)
	}

	return nil
}

type standardStreamSource struct {
	globalKeys [][16]byte
	totalCount int
	index      int
	rowValues  []any
	now        int64
}

func (s *standardStreamSource) Next() bool {
	if s.index >= s.totalCount {
		return false
	}
	var id [16]byte
	if s.index < len(s.globalKeys) {
		id = s.globalKeys[s.index]
	} else {
		uuidVal, err := googleuuid.NewV7()
		if err != nil {
			uuidVal = googleuuid.New()
		}
		id = uuidVal
	}

	s.rowValues[0] = id
	s.rowValues[1] = "Jean-Sébastien"
	s.rowValues[2] = "Bach"
	s.rowValues[3] = int64(-6468729600)
	s.rowValues[4] = true
	s.rowValues[5] = s.now
	s.rowValues[6] = s.now
	s.rowValues[7] = int64(0)

	s.index++
	return true
}

func (s *standardStreamSource) Values() ([]any, error) {
	return s.rowValues, nil
}

func (s *standardStreamSource) Err() error {
	return nil
}

// PopulateStandardTable populates users_standard with totalCount background records using high-performance CopyFrom.
func PopulateStandardTable(ctx context.Context, pool *pgxpool.Pool, globalKeys [][16]byte, totalCount int) error {
	var count int64
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM (SELECT 1 FROM users_standard LIMIT $1) t", totalCount).Scan(&count)
	if count >= int64(totalCount) {
		slog.Info("users_standard table already contains required background records. Skipping population.", "count", count)
		return nil
	}

	slog.Info("Populating users_standard with background records for realistic sizing...", "target", totalCount, "current", count)

	source := &standardStreamSource{
		globalKeys: globalKeys,
		totalCount: totalCount,
		rowValues:  make([]any, 8),
		now:        time.Now().Unix(),
	}

	popCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	_, err := pool.CopyFrom(
		popCtx,
		pgx.Identifier{"users_standard"},
		[]string{"id", "first_name", "last_name", "birth_date", "active", "created_at", "updated_at", "deleted_at"},
		source,
	)
	if err != nil {
		return fmt.Errorf("failed to populate users_standard: %w", err)
	}

	slog.Info("Analyzing users_standard table...")
	_, _ = pool.Exec(ctx, "ANALYZE users_standard")
	return nil
}
