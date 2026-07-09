package domain

import (
	"bytes"
	"testing"
)

func TestUserDataSerialization(t *testing.T) {
	uuid, err := NewUUID()
	if err != nil {
		t.Fatalf("failed to generate UUID: %v", err)
	}

	original := UserData{
		ID:        uuid,
		FirstName: "Jean-Sébastien",
		LastName:  "Bach",
		BirthDate: -6468729600, // 1685-03-31
		Active:    true,
		CreatedAt: 1716739200,
		UpdatedAt: 1716742800,
		DeletedAt: 0,
	}

	// Test original binary serialization
	dataBin, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	var decodedBin UserData
	err = decodedBin.UnmarshalBinary(dataBin)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}
	assertUserEqual(t, &original, &decodedBin)

	// Test manual Protobuf wire serialization
	dataProto, err := original.MarshalProtobuf()
	if err != nil {
		t.Fatalf("MarshalProtobuf failed: %v", err)
	}

	var decodedProto UserData
	err = decodedProto.UnmarshalProtobuf(dataProto)
	if err != nil {
		t.Fatalf("UnmarshalProtobuf failed: %v", err)
	}
	assertUserEqual(t, &original, &decodedProto)
}

func TestULIDHelpers(t *testing.T) {
	ulid, err := NewULID()
	if err != nil {
		t.Fatalf("failed to generate ULID: %v", err)
	}

	ulidStr := EncodeULID(ulid)
	if len(ulidStr) != 26 {
		t.Fatalf("invalid ULID string length: expected 26, got %d", len(ulidStr))
	}

	parsed, err := ParseULID(ulidStr)
	if err != nil {
		t.Fatalf("ParseULID failed: %v", err)
	}

	if !bytes.Equal(ulid[:], parsed[:]) {
		t.Errorf("ULID string roundtrip failed: original %x, parsed %x", ulid, parsed)
	}
}

func assertUserEqual(t *testing.T, expected, actual *UserData) {
	t.Helper()
	if !bytes.Equal(expected.ID[:], actual.ID[:]) {
		t.Errorf("ID mismatch: expected %v, got %v", expected.ID, actual.ID)
	}
	if expected.FirstName != actual.FirstName {
		t.Errorf("FirstName mismatch: expected %q, got %q", expected.FirstName, actual.FirstName)
	}
	if expected.LastName != actual.LastName {
		t.Errorf("LastName mismatch: expected %q, got %q", expected.LastName, actual.LastName)
	}
	if expected.BirthDate != actual.BirthDate {
		t.Errorf("BirthDate mismatch: expected %d, got %d", expected.BirthDate, actual.BirthDate)
	}
	if expected.Active != actual.Active {
		t.Errorf("Active mismatch: expected %t, got %t", expected.Active, actual.Active)
	}
	if expected.CreatedAt != actual.CreatedAt {
		t.Errorf("CreatedAt mismatch: expected %d, got %d", expected.CreatedAt, actual.CreatedAt)
	}
	if expected.UpdatedAt != actual.UpdatedAt {
		t.Errorf("UpdatedAt mismatch: expected %d, got %d", expected.UpdatedAt, actual.UpdatedAt)
	}
	if expected.DeletedAt != actual.DeletedAt {
		t.Errorf("DeletedAt mismatch: expected %d, got %d", expected.DeletedAt, actual.DeletedAt)
	}
}

func BenchmarkMarshalProtobuf(b *testing.B) {
	ulid, _ := NewULID()
	u := UserData{
		ID:        ulid,
		FirstName: "Christopher",
		LastName:  "Nolan",
		BirthDate: 191606400,
		Active:    true,
		CreatedAt: 1716739200,
		UpdatedAt: 1716742800,
		DeletedAt: 0,
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := u.MarshalProtobuf()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalProtobuf(b *testing.B) {
	ulid, _ := NewULID()
	u := UserData{
		ID:        ulid,
		FirstName: "Christopher",
		LastName:  "Nolan",
		BirthDate: 191606400,
		Active:    true,
		CreatedAt: 1716739200,
		UpdatedAt: 1716742800,
		DeletedAt: 0,
	}
	data, _ := u.MarshalProtobuf()

	var decoded UserData
	b.ResetTimer()
	for b.Loop() {
		err := decoded.UnmarshalProtobuf(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
