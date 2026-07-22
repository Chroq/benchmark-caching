package service

import (
	"context"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/port/input"
	"github.com/Chroq/benchmark-caching/internal/domain/port/output"
)

type UserService struct {
	repo output.UserRepository
}

// NewUserService instantiates a domain UserService wrapping a output UserRepository.
func NewUserService(repo output.UserRepository) input.UserUseCase {
	return &UserService{
		repo: repo,
	}
}

func (s *UserService) GetUser(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	return s.repo.Get(ctx, id, dest)
}

func (s *UserService) SetUser(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	return s.repo.Set(ctx, user, ttl)
}

func (s *UserService) Close() error {
	if s.repo != nil {
		return s.repo.Close()
	}
	return nil
}
