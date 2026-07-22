package postgresql

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/Chroq/benchmark-caching/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations connects to PostgreSQL and runs database schema migrations for the selected engine.
func RunMigrations(ctx context.Context, databaseURL string, engine string) error {
	slog.Info("Connecting to PostgreSQL to run migrations...")
	var migConn *pgx.Conn
	var err error
	for i := range 5 {
		connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
		migConn, err = pgx.Connect(connectCtx, databaseURL)
		connectCancel()
		if err == nil {
			break
		}
		slog.Info("PostgreSQL connection failed, retrying in 1s...", "attempt", i+1, "error", err)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("postgresql migration connection failed after retries: %w", err)
	}

	schemaFile := "internal/infrastructure/postgresql/optimized/schema.sql"
	if engine == "standard-postgresql" {
		schemaFile = "internal/infrastructure/postgresql/standard/schema.sql"
	}

	slog.Info("Executing database migrations...", "schemaFile", schemaFile)
	schemaBytes, err := os.ReadFile(schemaFile)
	if err != nil {
		schemaBytes, err = os.ReadFile("../../" + schemaFile)
		if err != nil {
			_ = migConn.Close(context.Background())
			return fmt.Errorf("failed to read %s: %w", schemaFile, err)
		}
	}

	execCtx, execCancel := context.WithTimeout(ctx, 60*time.Second)
	_, err = migConn.Exec(execCtx, string(schemaBytes))
	execCancel()
	if err != nil {
		_ = migConn.Close(context.Background())
		return fmt.Errorf("failed to execute database migrations: %w", err)
	}
	_ = migConn.Close(context.Background())
	slog.Info("Database migrations executed successfully.", "schemaFile", schemaFile)
	return nil
}

// NewPool initializes a pgxpool connection pool tailored for the selected engine.
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	if err := RunMigrations(ctx, cfg.DatabaseURL, cfg.Engine); err != nil {
		return nil, err
	}

	slog.Info("Initializing PostgreSQL Pool...")
	pgConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DATABASE_URL: %w", err)
	}

	maxConns := int32(cfg.DatabaseMaxConns)
	if maxConns <= 0 {
		goMaxProcsStr := os.Getenv("GOMAXPROCS")
		if goMaxProcsStr == "" {
			goMaxProcsStr = "4"
		}
		goMaxProcs, err := strconv.Atoi(goMaxProcsStr)
		if err != nil {
			slog.Error("failed to parse GOMAXPROCS, using default", "error", err)
			goMaxProcs = 4
		}
		maxConns = int32((goMaxProcs * 2) + 1)
	}

	pgConfig.MaxConns = maxConns
	pgConfig.MinConns = maxConns
	pgConfig.MaxConnIdleTime = 30 * time.Minute
	pgConfig.MaxConnLifetime = 5 * time.Minute

	if cfg.Engine == "standard-postgresql" {
		pgConfig.AfterConnect = func(connectCtx context.Context, conn *pgx.Conn) error {
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
	} else if cfg.Engine == "optimized-postgresql" {
		pgConfig.AfterConnect = func(connectCtx context.Context, conn *pgx.Conn) error {
			_, errOptGet := conn.Prepare(connectCtx, "get_user_optimized",
				"SELECT value FROM cache_optimized WHERE key = $1 AND expires_at > $2")
			_, errOptSet := conn.Prepare(connectCtx, "set_user_optimized",
				"INSERT INTO cache_optimized (key, value, expires_at) VALUES ($1, $2, $3) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at")

			if errOptGet != nil {
				return errOptGet
			}
			if errOptSet != nil {
				return errOptSet
			}
			return nil
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("postgresql pool setup failed: %w", err)
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := pool.Ping(connCtx); err != nil {
		cancel()
		pool.Close()
		return nil, fmt.Errorf("postgresql connection ping failed: %w", err)
	}
	cancel()

	return pool, nil
}
