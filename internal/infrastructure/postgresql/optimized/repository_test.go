package optimized_test

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/optimized"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	oklogulid "github.com/oklog/ulid/v2"
)

func TestOptimizedRepository(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	ctx := context.Background()

	// 1. Migration Setup
	migConn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("Skipping OptimizedRepository integration test (database connection failed: %v)", err)
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

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create pgxpool: %v", err)
	}
	defer pool.Close()

	repo := optimized.NewRepository(pool)

	_, err = pool.Exec(ctx, "SELECT purge_expired_cache_keys(10000)")
	if err != nil {
		t.Fatalf("Failed to execute purge_expired_cache_keys(10000): %v", err)
	}

	ulidVal := oklogulid.MustNew(oklogulid.Timestamp(time.Now()), rand.Reader)

	user := &model.UserData{
		ID:        model.ID(ulidVal),
		FirstName: "Optimized",
		LastName:  "User",
		BirthDate: 946684800,
		Active:    true,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		DeletedAt: 0,
	}

	err = repo.Set(ctx, user, 2*time.Hour)
	if err != nil {
		t.Fatalf("Optimized Set failed: %v", err)
	}

	var retrieved model.UserData
	found, err := repo.Get(ctx, model.ID(ulidVal), &retrieved)
	if err != nil {
		t.Fatalf("Optimized Get failed: %v", err)
	}
	if !found {
		t.Fatalf("Optimized Get failed: expected item to be found")
	}

	if retrieved.FirstName != "Optimized" || retrieved.LastName != "User" {
		t.Errorf("Optimized Get returned wrong payload: %+v", retrieved)
	}
}
