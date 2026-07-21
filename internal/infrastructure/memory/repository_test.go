package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/memory"
)

func TestOtterRepository_GetSet(t *testing.T) {
	cache := memory.NewCache()
	repo := memory.NewRepository(cache)
	defer repo.Close()
	ctx := context.Background()

	id := model.ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	user := &model.UserData{
		ID:        id,
		FirstName: "John",
		LastName:  "Doe",
		Active:    true,
	}

	err := repo.Set(ctx, user, 1*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	var fetched model.UserData
	found, err := repo.Get(ctx, id, &fetched)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatalf("expected to find key in cache")
	}
	if fetched.FirstName != "John" || fetched.LastName != "Doe" {
		t.Fatalf("unexpected fetched user content: %+v", fetched)
	}
}

func TestOtterRepository_TTLExpiration(t *testing.T) {
	cache := memory.NewCache()
	repo := memory.NewRepository(cache)
	defer repo.Close()
	ctx := context.Background()

	id := model.ID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	user := &model.UserData{
		ID:        id,
		FirstName: "Short",
		LastName:  "Lived",
	}

	err := repo.Set(ctx, user, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	var fetched model.UserData
	found, err := repo.Get(ctx, id, &fetched)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found {
		t.Fatalf("expected key to be expired")
	}
}


