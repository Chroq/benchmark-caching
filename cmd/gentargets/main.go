package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	oklogulid "github.com/oklog/ulid/v2"
)

func main() {
	count := flag.Int("count", 100000, "Number of keys to generate")
	outputKeys := flag.String("keys-file", "gen/keys.txt", "Output file for raw ULID keys")
	flag.Parse()

	keysDir := filepath.Dir(*outputKeys)
	if err := os.MkdirAll(keysDir, 0755); err != nil {
		fmt.Printf("Failed to create keys directory: %v\n", err)
		os.Exit(1)
	}

	entropy := oklogulid.Monotonic(rand.Reader, 0)

	keysFile, err := os.Create(*outputKeys)
	if err != nil {
		fmt.Printf("Failed to create keys file: %v\n", err)
		os.Exit(1)
	}
	defer keysFile.Close()

	keysWriter := bufio.NewWriter(keysFile)

	for i := 0; i < *count; i++ {
		id := oklogulid.MustNew(oklogulid.Timestamp(time.Now()), entropy)
		keyStr := id.String()

		_, err := fmt.Fprintf(keysWriter, "%s\n", keyStr)
		if err != nil {
			fmt.Printf("Failed to write key: %v\n", err)
			os.Exit(1)
		}
	}

	if err := keysWriter.Flush(); err != nil {
		fmt.Printf("Failed to flush keys writer: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %d raw keys in %s\n", *count, *outputKeys)
}
