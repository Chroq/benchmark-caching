package valkey

import (
	"context"
	"time"

	"github.com/Chroq/benchmark-caching/domain"
	"github.com/redis/go-redis/v9"
)

// Repository manages Valkey/Redis caching.
type Repository struct {
	client *redis.Client
}

// NewRepository creates a new Valkey repository instance.
func NewRepository(client *redis.Client) *Repository {
	return &Repository{
		client: client,
	}
}

// Get fetches a UserData using the ULID key into a destination struct.
func (r *Repository) Get(ctx context.Context, id [16]byte, dest *domain.UserData) (bool, error) {
	// Convert ULID to Crockford Base32 string key
	keyStr := domain.EncodeULID(id)

	val, err := r.client.Get(ctx, keyStr).Bytes()
	if err != nil {
		if err == redis.Nil {
			return false, nil // Cache miss
		}
		return false, err
	}

	if err := dest.UnmarshalProtobuf(val); err != nil {
		return false, err
	}

	return true, nil
}

// Set stores a UserData using the ULID key.
func (r *Repository) Set(ctx context.Context, user *domain.UserData, ttl time.Duration) error {
	keyStr := domain.EncodeULID(user.ID)

	val, err := user.MarshalProtobuf()
	if err != nil {
		return err
	}

	return r.client.Set(ctx, keyStr, val, ttl).Err()
}

// Seed bulk-populates Valkey with users using Redis Pipeline in batches of 50,000 to prevent high memory allocations and CPU/GC thrashing.
func (r *Repository) Seed(ctx context.Context, users []domain.UserData, ttl time.Duration) error {
	const batchSize = 50000
	for i := 0; i < len(users); i += batchSize {
		end := i + batchSize
		if end > len(users) {
			end = len(users)
		}

		pipe := r.client.Pipeline()
		for j := i; j < end; j++ {
			u := &users[j]
			keyStr := domain.EncodeULID(u.ID)
			val, err := u.MarshalProtobuf()
			if err != nil {
				return err
			}
			pipe.Set(ctx, keyStr, val, ttl)
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
