package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositories(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("failed to parse DATABASE_URL: %v", err)
	}

	config.AfterConnect = func(connectCtx context.Context, conn *pgx.Conn) error {
		// Prepare Standard Statements
		_, errStdGet := conn.Prepare(connectCtx, "get_user_standard",
			"SELECT id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at FROM users_standard WHERE id = $1 AND expires_at > $2")
		_, errStdSet := conn.Prepare(connectCtx, "set_user_standard",
			"INSERT INTO users_standard (id, first_name, last_name, birth_date, active, created_at, updated_at, deleted_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO UPDATE SET first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name, birth_date = EXCLUDED.birth_date, active = EXCLUDED.active, updated_at = EXCLUDED.updated_at, deleted_at = EXCLUDED.deleted_at, expires_at = EXCLUDED.expires_at")

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
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// 1. Test StandardRepository (Relational Database Table direct access)
	t.Run("StandardRepository", func(t *testing.T) {
		repo := NewStandardRepository(pool)

		uuidVal, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("NewUUID failed: %v", err)
		}

		user := &domain.UserData{
			ID:        uuidVal,
			FirstName: "Johann",
			LastName:  "Bach",
			BirthDate: -6468729600,
			Active:    true,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}

		// Ensure table clean
		_, err = pool.Exec(ctx, "DELETE FROM users_standard WHERE id = $1", uuidVal)
		if err != nil {
			t.Fatalf("clean failed: %v", err)
		}

		domain.CurrentTimeUTC.Store(time.Now().UTC())

		// Set
		err = repo.Set(ctx, user, 10*time.Second)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Get
		var fetched domain.UserData
		domain.CurrentTimeUTC.Store(time.Now().UTC())
		found, err := repo.Get(ctx, uuidVal, &fetched)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !found {
			t.Fatalf("expected to find user, but not found")
		}

		if fetched.FirstName != user.FirstName || fetched.LastName != user.LastName {
			t.Errorf("data mismatch: got %s %s, expected %s %s", fetched.FirstName, fetched.LastName, user.FirstName, user.LastName)
		}

		// Ensure table clean before Seed to avoid duplicate key conflict
		_, err = pool.Exec(ctx, "DELETE FROM users_standard WHERE id = $1", uuidVal)
		if err != nil {
			t.Fatalf("clean before seed failed: %v", err)
		}

		// Seed
		err = repo.Seed(ctx, []domain.UserData{*user}, 10*time.Second)
		if err != nil {
			t.Fatalf("Seed failed: %v", err)
		}
	})

	// 3. Test OptimizedRepository
	t.Run("OptimizedRepository", func(t *testing.T) {
		repo := NewOptimizedRepository(pool)

		domain.BootstrapTime = time.Now().UTC()
		_, err := pool.Exec(ctx, "SELECT manage_cache_partitions()")
		if err != nil {
			t.Fatalf("manage_cache_partitions failed: %v", err)
		}

		uuidVal, err := domain.NewUUID()
		if err != nil {
			t.Fatalf("NewUUID failed: %v", err)
		}

		user := &domain.UserData{
			ID:        uuidVal,
			FirstName: "Sebastien",
			LastName:  "Bach",
			BirthDate: -6468729600,
			Active:    true,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}

		domain.CurrentTimeUTC.Store(time.Now().UTC())

		err = repo.Set(ctx, user, 2*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		var fetched domain.UserData
		domain.CurrentTimeUTC.Store(time.Now().UTC())
		found, err := repo.Get(ctx, uuidVal, &fetched)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !found {
			t.Fatalf("expected to find user, but not found")
		}

		if fetched.FirstName != user.FirstName {
			t.Errorf("data mismatch: got %s, expected %s", fetched.FirstName, user.FirstName)
		}

		err = repo.Seed(ctx, []domain.UserData{*user}, 2*time.Hour)
		if err != nil {
			t.Fatalf("Seed failed: %v", err)
		}
	})
}
