package ports

import (
	"context"
	"time"

	"github.com/Chroq/benchmark-caching/domain"
)

// NaiveUserRepository defines the port for the naive caching implementation.
type NaiveUserRepository interface {
	Get(ctx context.Context, key string, dest *domain.UserData) (bool, error)
	Set(ctx context.Context, key string, user *domain.UserData, ttl time.Duration) error
	Seed(ctx context.Context, users []domain.UserData, ttl time.Duration) error
}

// OptimizedUserRepository defines the port for the optimized caching implementation.
type OptimizedUserRepository interface {
	Get(ctx context.Context, id [16]byte, dest *domain.UserData) (bool, error)
	Set(ctx context.Context, user *domain.UserData, ttl time.Duration) error
	Seed(ctx context.Context, users []domain.UserData, ttl time.Duration) error
}

// StandardUserRepository defines the port for the standard relational caching implementation.
type StandardUserRepository interface {
	Get(ctx context.Context, id [16]byte, dest *domain.UserData) (bool, error)
	Set(ctx context.Context, user *domain.UserData, ttl time.Duration) error
	Seed(ctx context.Context, users []domain.UserData, ttl time.Duration) error
}

