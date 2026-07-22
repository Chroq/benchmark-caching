package tsid_test

import (
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	tsidrepo "github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/tsid"
	"github.com/Chroq/benchmark-caching/pkg/tsid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTSIDRepository(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	ctx := context.Background()

	// 1. Migration Setup
	migConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping TSIDRepository integration test (database connection failed: %v)", err)
		return
	}

	schemaBytes, err := os.ReadFile("schema.sql")
	if err != nil {
		migConn.Close(ctx)
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	_, err = migConn.Exec(ctx, string(schemaBytes))
	migConn.Close(ctx)
	if err != nil {
		t.Fatalf("Failed to run migrations from schema.sql: %v", err)
	}

	// 2. Pool Setup
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("Failed to parse DATABASE_URL: %v", err)
	}

	config.AfterConnect = func(connectCtx context.Context, conn *pgx.Conn) error {
		_, errTsidGet := conn.Prepare(connectCtx, "get_user_tsid",
			"SELECT id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at FROM users_tsid WHERE id = $1")
		_, errTsidSet := conn.Prepare(connectCtx, "set_user_tsid",
			"INSERT INTO users_tsid (id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO UPDATE SET first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, birth_date = EXCLUDED.birth_date, active = EXCLUDED.active, updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at")

		if errTsidGet != nil {
			return errTsidGet
		}
		if errTsidSet != nil {
			return errTsidSet
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pgxpool: %v", err)
	}
	defer pool.Close()

	repo := tsidrepo.NewRepository(pool)

	rawTSID := tsid.New()
	var idBytes [16]byte
	binary.BigEndian.PutUint64(idBytes[:8], uint64(rawTSID))

	user := &model.UserData{
		ID:        model.ID(idBytes),
		FirstName: "TSID",
		LastName:  "User",
		BirthDate: 946684800,
		Active:    true,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		DeletedAt: 0,
	}

	err = repo.Set(ctx, user, 8*time.Hour)
	if err != nil {
		t.Fatalf("TSID Set failed: %v", err)
	}

	var retrieved model.UserData
	found, err := repo.Get(ctx, model.ID(idBytes), &retrieved)
	if err != nil {
		t.Fatalf("TSID Get failed: %v", err)
	}
	if !found {
		t.Fatalf("TSID Get failed: expected item to be found")
	}

	if retrieved.FirstName != "TSID" || retrieved.LastName != "User" {
		t.Errorf("TSID Get returned wrong payload: %+v", retrieved)
	}
}
