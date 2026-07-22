package serializer_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/Chroq/benchmark-caching/internal/domain/model"
	"github.com/Chroq/benchmark-caching/internal/infrastructure/serializer"
	oklogulid "github.com/oklog/ulid/v2"
)

func TestProtobufSerialization(t *testing.T) {
	id := oklogulid.MustNew(oklogulid.Timestamp(time.Now()), rand.Reader)

	now := time.Now().Unix()
	user := model.UserData{
		ID:        model.ID(id),
		FirstName: "Jean-Sébastien",
		LastName:  "Bach",
		BirthDate: -6468729600,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
		DeletedAt: 0,
	}

	data, err := serializer.MarshalProtobuf(&user)
	if err != nil {
		t.Fatalf("MarshalProtobuf failed: %v", err)
	}

	var decoded model.UserData
	if err := serializer.UnmarshalProtobuf(data, &decoded); err != nil {
		t.Fatalf("UnmarshalProtobuf failed: %v", err)
	}

	if decoded.ID != user.ID {
		t.Errorf("ID mismatch: expected %x, got %x", user.ID, decoded.ID)
	}
	if decoded.FirstName != user.FirstName {
		t.Errorf("FirstName mismatch: expected %s, got %s", user.FirstName, decoded.FirstName)
	}
	if decoded.LastName != user.LastName {
		t.Errorf("LastName mismatch: expected %s, got %s", user.LastName, decoded.LastName)
	}
	if decoded.BirthDate != user.BirthDate {
		t.Errorf("BirthDate mismatch: expected %d, got %d", user.BirthDate, decoded.BirthDate)
	}
	if decoded.Active != user.Active {
		t.Errorf("Active mismatch: expected %v, got %v", user.Active, decoded.Active)
	}
}
