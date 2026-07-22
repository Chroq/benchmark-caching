package tsid

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
	"github.com/Chroq/benchmark-caching/pkg/tsid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TSIDRepository implements standard relational storage with 64-bit TSID BIGINT primary key.
type TSIDRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new TSID PostgreSQL repository.
func NewRepository(pool *pgxpool.Pool) output.UserRepository {
	return &TSIDRepository{
		pool: pool,
	}
}

// Close closes the underlying PostgreSQL connection pool.
func (r *TSIDRepository) Close() error {
	if r.pool != nil {
		r.pool.Close()
	}
	return nil
}

// Get retrieves a UserData by its 64-bit TSID key from the relational table users_tsid.
func (r *TSIDRepository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	tsidKey := int64(binary.BigEndian.Uint64(id[:8]))
	var retrievedID int64

	err := r.pool.QueryRow(ctx,
		"get_user_tsid",
		tsidKey,
	).Scan(
		&retrievedID,
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
		return false, fmt.Errorf("tsid get db error: %w", err)
	}

	binary.BigEndian.PutUint64(dest.ID[:8], uint64(retrievedID))
	return true, nil
}

// Set stores a UserData by writing directly to standard SQL columns with a 64-bit TSID BIGINT key.
func (r *TSIDRepository) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	tsidKey := int64(binary.BigEndian.Uint64(user.ID[:8]))

	_, err := r.pool.Exec(ctx,
		"set_user_tsid",
		tsidKey,
		user.FirstName,
		user.LastName,
		user.BirthDate,
		user.Active,
		user.CreatedAt,
		user.UpdatedAt,
		user.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("tsid set db error: %w", err)
	}

	return nil
}

type tsidStreamSource struct {
	globalKeys [][16]byte
	rowValues  []any
	totalCount int
	index      int
	startTime  int64
	timeSpan   int64
}

func (s *tsidStreamSource) Next() bool {
	if s.index >= s.totalCount {
		return false
	}

	var createdAt int64
	if s.totalCount > 1 {
		createdAt = s.startTime + (int64(s.index) * s.timeSpan / int64(s.totalCount))
	} else {
		createdAt = s.startTime
	}

	var tsidVal int64
	if s.index < len(s.globalKeys) {
		tsidVal = int64(binary.BigEndian.Uint64(s.globalKeys[s.index][:8]))
	} else {
		tsidVal = tsid.NewFromTimeMs(createdAt * 1000)
	}

	s.rowValues[0] = tsidVal
	s.rowValues[1] = "Jean-Sébastien"
	s.rowValues[2] = "Bach"
	s.rowValues[3] = int64(-6468729600)
	s.rowValues[4] = false
	s.rowValues[5] = createdAt
	s.rowValues[6] = createdAt
	s.rowValues[7] = int64(0)

	s.index++
	return true
}

func (s *tsidStreamSource) Values() ([]any, error) {
	return s.rowValues, nil
}

func (s *tsidStreamSource) Err() error {
	return nil
}

// PopulateTSIDTable populates users_tsid with totalCount background records using high-performance CopyFrom.
func PopulateTSIDTable(ctx context.Context, pool *pgxpool.Pool, globalKeys [][16]byte, totalCount int) error {
	slog.Info("Cleaning up previous benchmark test records (active = true)...")
	tag, err := pool.Exec(ctx, "DELETE FROM users_tsid WHERE active = true")
	if err != nil {
		slog.Warn("Failed to cleanup active test records from users_tsid", "error", err)
	} else if tag.RowsAffected() > 0 {
		slog.Info("Cleaned up previous test records from users_tsid", "deletedCount", tag.RowsAffected())
	}

	var count int64
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM (SELECT 1 FROM users_tsid LIMIT $1) t", totalCount).Scan(&count)
	if count >= int64(totalCount) {
		slog.Info("users_tsid table already contains required background records. Skipping population.", "count", count)
		return nil
	}

	slog.Info("Populating users_tsid with background records for realistic sizing...", "target", totalCount, "current", count)

	now := time.Now().Unix()
	fiveYearsAgo := now - (5 * 365 * 86400)
	source := &tsidStreamSource{
		globalKeys: globalKeys,
		totalCount: totalCount,
		rowValues:  make([]any, 8),
		startTime:  fiveYearsAgo,
		timeSpan:   now - fiveYearsAgo,
	}

	popCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	_, err = pool.CopyFrom(
		popCtx,
		pgx.Identifier{"users_tsid"},
		[]string{"id", "first_name", "last_name", "birth_date", "active", "created_at", "updated_at", "deleted_at"},
		source,
	)
	if err != nil {
		return fmt.Errorf("failed to populate users_tsid: %w", err)
	}

	slog.Info("Analyzing users_tsid table...")
	_, _ = pool.Exec(ctx, "ANALYZE users_tsid")
	return nil
}
