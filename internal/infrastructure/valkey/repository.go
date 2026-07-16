package valkey

import (
	"context"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/serializer"
	"github.com/Chroq/benchmark-caching/pkg/ulid"
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
func (r *Repository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	keyStr := ulid.EncodeULID(id)

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
	keyStr := ulid.EncodeULID(user.ID)

	val, err := serializer.MarshalProtobuf(user)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, keyStr, val, ttl).Err()
}
