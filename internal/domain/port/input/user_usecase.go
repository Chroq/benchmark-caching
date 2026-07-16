package input

import (
	"context"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
)

// UserUseCase defines the driving input port interface for user caching operations.
type UserUseCase interface {
	GetUser(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error)
	SetUser(ctx context.Context, user *model.UserData, ttl time.Duration) error
}
