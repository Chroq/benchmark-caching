package config_test

import (
	"os"
	"testing"

	"github.com/Chroq/benchmark-caching/internal/config"
)

func TestLoadConfig(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test?sslmode=disable")
	os.Setenv("DATABASE_MAX_CONNS", "128")
	os.Setenv("VALKEY_URL", "localhost:6380")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("DATABASE_MAX_CONNS")
		os.Unsetenv("VALKEY_URL")
	}()

	cfg := config.LoadConfig()

	if cfg.Port != "9090" {
		t.Errorf("expected Port 9090, got %s", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://test:test@localhost:5432/test?sslmode=disable" {
		t.Errorf("unexpected DatabaseURL: %s", cfg.DatabaseURL)
	}
	if cfg.DatabaseMaxConns != 128 {
		t.Errorf("expected DatabaseMaxConns 128, got %d", cfg.DatabaseMaxConns)
	}
	if cfg.ValkeyURL != "localhost:6380" {
		t.Errorf("expected ValkeyURL localhost:6380, got %s", cfg.ValkeyURL)
	}
}
