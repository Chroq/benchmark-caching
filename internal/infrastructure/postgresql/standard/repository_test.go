package standard_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/standard"
	googleuuid "github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStandardRepository(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	ctx := context.Background()

	// 1. Migration Setup
	migConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping StandardRepository integration test (database connection failed: %v)", err)
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

	// 2. Pool Setup with Connection-Level Prepared Statements
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("Failed to parse DATABASE_URL: %v", err)
	}

	config.AfterConnect = func(connectCtx context.Context, conn *pgx.Conn) error {
		_, errStdGet := conn.Prepare(connectCtx, "get_user_standard",
			"SELECT id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at FROM users_standard WHERE id = $1")
		_, errStdSet := conn.Prepare(connectCtx, "set_user_standard",
			"INSERT INTO users_standard (id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO UPDATE SET first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, birth_date = EXCLUDED.birth_date, active = EXCLUDED.active, updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at")

		if errStdGet != nil {
			return errStdGet
		}
		if errStdSet != nil {
			return errStdSet
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pgxpool: %v", err)
	}
	defer pool.Close()

	repo := standard.NewRepository(pool)

	uuidVal := googleuuid.New()

	user := &model.UserData{
		ID:        model.ID(uuidVal),
		FirstName: "Standard",
		LastName:  "User",
		BirthDate: 946684800,
		Active:    true,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		DeletedAt: 0,
	}

	err = repo.Set(ctx, user, 8*time.Hour)
	if err != nil {
		t.Fatalf("Standard Set failed: %v", err)
	}

	var retrieved model.UserData
	found, err := repo.Get(ctx, model.ID(uuidVal), &retrieved)
	if err != nil {
		t.Fatalf("Standard Get failed: %v", err)
	}
	if !found {
		t.Fatalf("Standard Get failed: expected item to be found")
	}

	if retrieved.FirstName != "Standard" || retrieved.LastName != "User" {
		t.Errorf("Standard Get returned wrong payload: %+v", retrieved)
	}
}
