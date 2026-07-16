package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/Chroq/benchmark-caching/internal/config"
	"github.com/Chroq/benchmark-caching/internal/domain/port/input"
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
	"github.com/Chroq/benchmark-caching/internal/domain/service"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/memory"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/optimized"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/standard"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/valkey"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// SetupUseCase initializes storage engines based on active selection and returns the wired UserUseCase.
func SetupUseCase(ctx context.Context, cfg *config.Config, globalKeys [][16]byte) (input.UserUseCase, error) {
	runtime.GC()

	var repo output.UserRepository

	switch cfg.Engine {
	case "memory":
		slog.Info("Initializing In-Memory repository...")
		repo = memory.NewRepository()

	case "valkey":
		slog.Info("Initializing Valkey Client Pool (PoolSize=500)...")
		rdb := redis.NewClient(&redis.Options{
			Addr:         cfg.ValkeyURL,
			PoolSize:     2500,
			MinIdleConns: 200,
			ReadTimeout:  5 * time.Minute,
			WriteTimeout: 5 * time.Minute,
		})

		var pingErr error
		for i := range 5 {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			pingErr = rdb.Ping(pingCtx).Err()
			cancel()
			if pingErr == nil {
				break
			}
			slog.Info("Valkey connection failed, retrying in 1s...", "attempt", i+1, "error", pingErr)
			time.Sleep(1 * time.Second)
		}
		if pingErr != nil {
			return nil, fmt.Errorf("valkey connection failed after retries: %w", pingErr)
		}

		repo = valkey.NewRepository(rdb)

	case "optimized-postgresql", "standard-postgresql":
		slog.Info("Connecting to PostgreSQL to run migrations...")
		var migConn *pgx.Conn
		var err error
		for i := range 5 {
			connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
			migConn, err = pgx.Connect(connectCtx, cfg.DatabaseURL)
			connectCancel()
			if err == nil {
				break
			}
			slog.Info("PostgreSQL connection failed, retrying in 1s...", "attempt", i+1, "error", err)
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			return nil, fmt.Errorf("postgresql migration connection failed after retries: %w", err)
		}

		schemaFile := "internal/infrastructure/postgresql/optimized/schema.sql"
		if cfg.Engine == "standard-postgresql" {
			schemaFile = "internal/infrastructure/postgresql/standard/schema.sql"
		}

		slog.Info("Executing database migrations...", "schemaFile", schemaFile)
		schemaBytes, err := os.ReadFile(schemaFile)
		if err != nil {
			schemaBytes, err = os.ReadFile("../../" + schemaFile)
			if err != nil {
				migConn.Close(context.Background())
				return nil, fmt.Errorf("failed to read %s: %w", schemaFile, err)
			}
		}

		execCtx, execCancel := context.WithTimeout(ctx, 60*time.Second)
		_, err = migConn.Exec(execCtx, string(schemaBytes))
		execCancel()
		if err != nil {
			migConn.Close(context.Background())
			return nil, fmt.Errorf("failed to execute database migrations: %w", err)
		}
		migConn.Close(context.Background())
		slog.Info("Database migrations executed successfully.", "schemaFile", schemaFile)

		slog.Info("Initializing PostgreSQL Pool (MaxConns=25, MinConns=25)...")
		pgConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DATABASE_URL: %w", err)
		}

		goMaxProcsStr := os.Getenv("GOMAXPROCS")
		if goMaxProcsStr == "" {
			goMaxProcsStr = "4"
		}
		goMaxProcs, err := strconv.Atoi(goMaxProcsStr)
		if err != nil {
			slog.Error("failed to parse GOMAXPROCS, using default", "error", err)
			goMaxProcs = 4
		}
		pgConfig.MaxConns = int32((goMaxProcs * 2) + 1)
		pgConfig.MinConns = int32((goMaxProcs * 2) + 1)
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

		switch cfg.Engine {
		case "standard-postgresql":
			repo = standard.NewRepository(pool)
		case "optimized-postgresql":
			repo = optimized.NewRepository(pool)
		}

	default:
		return nil, fmt.Errorf("unsupported engine: %s", cfg.Engine)
	}

	return service.NewUserService(repo), nil
}
