package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Chroq/benchmark-caching/internal/config"
	"github.com/Chroq/benchmark-caching/internal/domain/port/input"
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
	"github.com/Chroq/benchmark-caching/internal/domain/service"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/memory"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/optimized"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/postgresql/standard"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/valkey"
)

// SetupUseCase initializes storage engine dependencies, injects them into the repository,
// wires the domain UserUseCase service, and returns a cleanup closure for graceful shutdown.
func SetupUseCase(ctx context.Context, cfg *config.Config, globalKeys [][16]byte) (input.UserUseCase, func(), error) {
	var repo output.UserRepository

	switch cfg.Engine {
	case "memory":
		slog.Info("Initializing In-Memory repository with injected Otter cache...")
		cache := memory.NewCache()
		repo = memory.NewRepository(cache)

	case "valkey":
		slog.Info("Initializing Valkey client pool...")
		client, err := valkey.NewClient(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to instantiate Valkey client: %w", err)
		}
		repo = valkey.NewRepository(client)

	case "optimized-postgresql", "standard-postgresql":
		slog.Info("Initializing PostgreSQL pool with migrations...", "engine", cfg.Engine)
		pool, err := postgresql.NewPool(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to instantiate PostgreSQL pool: %w", err)
		}

		if cfg.Engine == "standard-postgresql" {
			repo = standard.NewRepository(pool)
			if err := standard.PopulateStandardTable(ctx, pool, globalKeys, 10_000_000); err != nil {
				_ = repo.Close()
				return nil, nil, fmt.Errorf("failed to populate standard postgresql dataset: %w", err)
			}
		} else {
			repo = optimized.NewRepository(pool)
		}

	default:
		return nil, nil, fmt.Errorf("unsupported engine: %s", cfg.Engine)
	}

	useCase := service.NewUserService(repo)

	cleanup := func() {
		slog.Info("Closing repository connections...", "engine", cfg.Engine)
		if err := repo.Close(); err != nil {
			slog.Error("Error closing repository connections", "engine", cfg.Engine, "error", err)
		} else {
			slog.Info("Repository connections closed successfully.", "engine", cfg.Engine)
		}
	}

	return useCase, cleanup, nil
}
