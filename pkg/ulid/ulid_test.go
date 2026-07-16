package ulid_test

import (
	"bytes"
	"testing"

	"github.com/Chroq/benchmark-caching/pkg/ulid"
)

func TestGenerateULIDMonotonic(t *testing.T) {
	generator, err := ulid.NewULIDGenerator()
	if err != nil {
		t.Fatalf("NewULIDGenerator failed: %v", err)
	}

	prev := generator.GenerateULID()
	for i := range 10000 {
		curr := generator.GenerateULID()
		if bytes.Compare(prev[:], curr[:]) >= 0 {
			t.Fatalf("ULIDs are not monotonic at iteration %d: prev %x, curr %x", i, prev, curr)
		}
		prev = curr
	}
}

func TestULIDEncodeParseRoundtrip(t *testing.T) {
	gen, err := ulid.NewULIDGenerator()
	if err != nil {
		t.Fatalf("NewULIDGenerator failed: %v", err)
	}

	for range 100 {
		original := gen.GenerateULID()
		encoded := ulid.EncodeULID(original)
		parsed, err := ulid.ParseULID(encoded)
		if err != nil {
			t.Fatalf("ParseULID failed for %s: %v", encoded, err)
		}
		if parsed != original {
			t.Fatalf("Mismatch: original=%x, parsed=%x, encoded=%s", original, parsed, encoded)
		}
	}
}
