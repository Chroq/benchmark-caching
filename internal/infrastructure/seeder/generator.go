package seeder

import (
	"bufio"
	"log/slog"
	"os"
	"strings"

	googleuuid "github.com/google/uuid"
	oklogulid "github.com/oklog/ulid/v2"
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
			if len(line) == 26 {
				if key, err := oklogulid.Parse(line); err == nil {
					globalKeys = append(globalKeys, key)
				}
			} else if len(line) == 36 {
				if key, err := googleuuid.Parse(line); err == nil {
					globalKeys = append(globalKeys, key)
				}
			}
		}
		_ = file.Close()
		slog.Info("Loaded keys successfully", "count", len(globalKeys), "path", keysPath)
	}

	remaining := count - len(globalKeys)
	if remaining > 0 {
		slog.Info("Generating remaining UUID v7 cache entries...", "count", remaining)
		for range remaining {
			id, err := googleuuid.NewV7()
			if err != nil {
				id = googleuuid.New()
			}
			globalKeys = append(globalKeys, id)
		}
	}

	return globalKeys
}
