package tsid_test

import (
	"testing"

	"github.com/Chroq/benchmark-caching/pkg/tsid"
)

func TestTSIDGeneration(t *testing.T) {
	id1 := tsid.New()
	id2 := tsid.New()

	if id1 <= 0 {
		t.Errorf("expected positive TSID, got %d", id1)
	}

	if id2 <= id1 {
		t.Errorf("expected monotonic increasing TSID: id1=%d, id2=%d", id1, id2)
	}

	formatted := tsid.FormatInt(id1)
	parsed, err := tsid.ParseInt(formatted)
	if err != nil {
		t.Fatalf("ParseInt failed on formatted TSID %s: %v", formatted, err)
	}

	if parsed != id1 {
		t.Errorf("mismatch parsed TSID: expected %d, got %d", id1, parsed)
	}
}
