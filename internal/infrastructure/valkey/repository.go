package valkey

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Chroq/benchmark-caching/internal/config"
	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/serializer"
	oklogulid "github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
)

// Repository manages Valkey/Redis caching.
type Repository struct {
	client *redis.Client
}

// NewClient initializes a Valkey client pool with connection retry support.
func NewClient(ctx context.Context, cfg *config.Config) (*redis.Client, error) {
	slog.Info("Initializing Valkey Client Pool (PoolSize=2500)...")
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
		_ = rdb.Close()
		return nil, fmt.Errorf("valkey connection failed after retries: %w", pingErr)
	}

	return rdb, nil
}

// NewRepository creates a new Valkey repository instance using the injected client.
func NewRepository(client *redis.Client) *Repository {
	return &Repository{
		client: client,
	}
}

// Close closes the underlying Valkey client pool.
func (r *Repository) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Get fetches a UserData using the ULID key into a destination struct.
func (r *Repository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	keyStr := oklogulid.ULID(id).String()

	val, err := r.client.Get(ctx, keyStr).Bytes()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}

	if err := serializer.UnmarshalProtobuf(val, dest); err != nil {
		return false, err
	}

	return true, nil
}

// Set stores a UserData using the ULID key.
func (r *Repository) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	keyStr := oklogulid.ULID(user.ID).String()

	val, err := serializer.MarshalProtobuf(user)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, keyStr, val, ttl).Err()
}
