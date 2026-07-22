package valkey_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/config"
	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/valkey"
	googleuuid "github.com/google/uuid"
)

func TestValkeyRepository(t *testing.T) {
	valkeyURL := os.Getenv("VALKEY_URL")
	if valkeyURL == "" {
		valkeyURL = "127.0.0.1:6379"
	}

	cfg := &config.Config{
		ValkeyURL: valkeyURL,
	}

	ctx := context.Background()
	client, err := valkey.NewClient(ctx, cfg)
	if err != nil {
		t.Skipf("Skipping ValkeyRepository integration test (connection failed: %v)", err)
		return
	}
	defer client.Close()

	repo := valkey.NewRepository(client)
	defer repo.Close()

	id, err := googleuuid.NewV7()
	if err != nil {
		t.Fatalf("Failed to generate UUID v7: %v", err)
	}
	user := &model.UserData{
		ID:        model.ID(id),
		FirstName: "Valkey",
		LastName:  "User",
		BirthDate: 946684800,
		Active:    true,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		DeletedAt: 0,
	}

	err = repo.Set(ctx, user, 10*time.Minute)
	if err != nil {
		t.Fatalf("Valkey Set failed: %v", err)
	}

	var retrieved model.UserData
	found, err := repo.Get(ctx, model.ID(id), &retrieved)
	if err != nil {
		t.Fatalf("Valkey Get failed: %v", err)
	}
	if !found {
		t.Fatalf("Valkey Get failed: expected item to be found")
	}

	if retrieved.FirstName != "Valkey" || retrieved.LastName != "User" {
		t.Errorf("Valkey Get returned wrong payload: %+v", retrieved)
	}
}
