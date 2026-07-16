package seeder

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Chroq/benchmark-caching/pkg/ulid"
)

// GenerateKeys constructs the dataset of distinct ULID keys.
// If gen/keys.txt (or keys.txt) exists, it loads keys from it to ensure exact match with Vegeta targets.
func GenerateKeys(count int) [][16]byte {
	globalKeys := make([][16]byte, 0, count)

	keysPath := "gen/keys.txt"
	file, err := os.Open(keysPath)
	if err != nil {
		keysPath = "keys.txt"
		file, err = os.Open(keysPath)
	}

	if err == nil {
		slog.Info("Loading pre-generated ULID keys...", "path", keysPath)
		scanner := bufio.NewScanner(file)
		for scanner.Scan() && len(globalKeys) < count {
			line := strings.TrimSpace(scanner.Text())
			if len(line) == 0 {
				continue
			}
			if key, err := ulid.ParseULID(line); err == nil {
				globalKeys = append(globalKeys, key)
			}
		}
		_ = file.Close()
		slog.Info("Loaded keys successfully", "count", len(globalKeys), "path", keysPath)
	}

	remaining := count - len(globalKeys)
	if remaining > 0 {
		slog.Info("Generating remaining ULID-based cache entries...", "count", remaining)
		generator, err := ulid.NewULIDGenerator()
		if err != nil {
			panic(fmt.Sprintf("failed to create ULID generator: %v", err))
		}

		for range remaining {
			ulidVal := generator.GenerateULID()
			globalKeys = append(globalKeys, ulidVal)
		}
	}

	return globalKeys
}
