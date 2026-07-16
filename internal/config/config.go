package config

import (
	"flag"
	"os"
)

type Config struct {
	Engine      string
	LogLevel    string
	Port        string
	DatabaseURL string
	ValkeyURL   string
}

// LoadConfig parses command line flags and environment variables into Config.
func LoadConfig() *Config {
	engine := flag.String("engine", "optimized-postgresql", "Storage engine to benchmark (memory, valkey, optimized-postgresql, standard-postgresql)")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error, production)")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
	}

	valkeyURL := os.Getenv("VALKEY_URL")
	if valkeyURL == "" {
		valkeyURL = "127.0.0.1:6379"
	}

	return &Config{
		Engine:      *engine,
		LogLevel:    *logLevel,
		Port:        port,
		DatabaseURL: dbURL,
		ValkeyURL:   valkeyURL,
	}
}
