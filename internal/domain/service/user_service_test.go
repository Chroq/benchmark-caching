package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/domain/service"
)

type mockRepo struct {
	store map[model.ID]model.UserData
}

func newMockRepo() *mockRepo {
	return &mockRepo{store: make(map[model.ID]model.UserData)}
}

func (m *mockRepo) Get(ctx context.Context, id [16]byte, dest *model.UserData) (bool, error) {
	val, ok := m.store[model.ID(id)]
	if !ok {
		return false, nil
	}
	*dest = val
	return true, nil
}

func (m *mockRepo) Set(ctx context.Context, user *model.UserData, ttl time.Duration) error {
	m.store[user.ID] = *user
	return nil
}

func (m *mockRepo) Close() error {
	return nil
}

func TestUserService(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)
	defer svc.Close()

	ctx := context.Background()
	id := model.ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	user := &model.UserData{
		ID:        id,
		FirstName: "Alice",
		LastName:  "Smith",
		Active:    true,
	}

	err := svc.SetUser(ctx, user, 1*time.Hour)
	if err != nil {
		t.Fatalf("SetUser failed: %v", err)
	}

	var retrieved model.UserData
	found, err := svc.GetUser(ctx, id, &retrieved)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if !found {
		t.Fatalf("expected user to be found")
	}
	if retrieved.FirstName != "Alice" || retrieved.LastName != "Smith" {
		t.Errorf("unexpected user data: %+v", retrieved)
	}
}
