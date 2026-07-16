package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Chroq/benchmark-caching/internal/bootstrap"
	"github.com/Chroq/benchmark-caching/internal/config"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/httpsrv"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/seeder"
	"github.com/valyala/fasthttp"
)

const nbKey = 100_000

func main() {
	// 1. Load configuration and CLI flags
	cfg := config.LoadConfig()

	// 2. Configure structured logger
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "production":
		level = slog.LevelWarn
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	slog.Info("Benchmarking engine selected", "engine", cfg.Engine, "logLevel", cfg.LogLevel)

	// 3. Generate ULID keys dataset
	globalKeys := seeder.GenerateKeys(nbKey)

	// 4. Setup storage engine repository & instantiate domain UserUseCase service
	ctx := context.Background()
	userUseCase, err := bootstrap.SetupUseCase(ctx, cfg, globalKeys)
	if err != nil {
		slog.Error("Failed to setup benchmark use case", "error", err)
		os.Exit(1)
	}

	// 5. Wire HTTP Primary Adapter using the driving UserUseCase
	handler := httpsrv.NewHandler(cfg.Engine, globalKeys, userUseCase)

	server := &fasthttp.Server{
		Handler:      handler.Handle,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 6. Graceful Shutdown listener
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	slog.Info("Bootstrapping complete. Cache benchmark HTTP server listening", "port", cfg.Port)

	go func() {
		if err := server.ListenAndServe(":" + cfg.Port); err != nil {
			slog.Error("FastHTTP server failure", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutdown signal received. Stopping FastHTTP server gracefully...")

	if err := server.Shutdown(); err != nil {
		slog.Error("FastHTTP graceful shutdown failed", "error", err)
	}

	slog.Info("Shutdown complete.")
}
