package output

import (
	"context"
	"io"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
)

// UserRepository defines the output port interface for all cache repositories.
type UserRepository interface {
	io.Closer
	Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error)
	Set(ctx context.Context, user *model.UserData, ttl time.Duration) error
}

// OptimizedUserRepository defines the output port for the optimized caching implementation.
type OptimizedUserRepository interface {
	UserRepository
}

// StandardUserRepository defines the output port for the standard relational caching implementation.
type StandardUserRepository interface {
	UserRepository
}
