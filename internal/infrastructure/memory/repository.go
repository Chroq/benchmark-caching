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

// NewCache instantiates a configured Otter cache instance.
func NewCache() *otter.Cache[model.ID, *model.UserData] {
	return otter.Must(&otter.Options[model.ID, *model.UserData]{
		InitialCapacity:  150_000, // 100_000 + 50_000 to compensate for eviction
		ExpiryCalculator: otter.ExpiryWriting[model.ID, *model.UserData](1 * time.Hour),
	})
}

// NewRepository creates a new in-memory repository instance with the injected Otter cache.
func NewRepository(cache *otter.Cache[model.ID, *model.UserData]) *Repository {
	if cache == nil {
		cache = NewCache()
	}
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

// Close cleans up memory resources and stops Otter cache background tasks.
func (r *Repository) Close() error {
	if r.cache != nil {
		r.cache.InvalidateAll()
		r.cache.StopAllGoroutines()
	}
	return nil
}
