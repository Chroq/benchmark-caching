package memory

import (
	"context"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/maypok86/otter/v2"
)

// Repository manages In-Memory caching using Otter.
type Repository struct {
	cache *otter.Cache[model.ID, *model.UserData]
}

// NewRepository creates a new in-memory repository instance using Otter cache.
func NewRepository() *Repository {
	cache := otter.Must(&otter.Options[model.ID, *model.UserData]{
		InitialCapacity:  100_000,
		ExpiryCalculator: otter.ExpiryWriting[model.ID, *model.UserData](1 * time.Hour),
	})

	return &Repository{
		cache: cache,
	}
}

func (r *Repository) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	val, ok := r.cache.GetIfPresent(model.ID(id))
	if !ok {
		return false, nil
	}
	*dest = *val
	return true, nil
}

func (r *Repository) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	u := *user
	r.cache.Set(user.ID, &u)
	if ttl > 0 {
		r.cache.SetExpiresAfter(user.ID, ttl)
	}
	return nil
}
