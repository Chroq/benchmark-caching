package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chroq/benchmark-caching/pkg/ulid"
)

func main() {
	count := flag.Int("count", 100000, "Number of keys/targets to generate")
	port := flag.String("port", "8080", "Target server port")
	outputKeys := flag.String("keys-file", "gen/keys.txt", "Output file for raw ULID keys")
	outputTargets := flag.String("targets-dir", "gen", "Output directory for vegeta target files")
	flag.Parse()

	if err := os.MkdirAll(*outputTargets, 0755); err != nil {
		fmt.Printf("Failed to create target directory: %v\n", err)
		os.Exit(1)
	}

	keysDir := filepath.Dir(*outputKeys)
	if err := os.MkdirAll(keysDir, 0755); err != nil {
		fmt.Printf("Failed to create keys directory: %v\n", err)
		os.Exit(1)
	}

	gen, err := ulid.NewULIDGenerator()
	if err != nil {
		fmt.Printf("Failed to create ULID generator: %v\n", err)
		os.Exit(1)
	}

	keysFile, err := os.Create(*outputKeys)
	if err != nil {
		fmt.Printf("Failed to create keys file: %v\n", err)
		os.Exit(1)
	}
	defer keysFile.Close()

	engines := []string{"memory", "valkey", "postgres"}
	getWriters := make(map[string]*bufio.Writer)
	setWriters := make(map[string]*bufio.Writer)

	for _, eng := range engines {
		fGet, err := os.Create(filepath.Join(*outputTargets, fmt.Sprintf("targets_%s_get.txt", eng)))
		if err != nil {
			fmt.Printf("Failed to create targets GET file for %s: %v\n", eng, err)
			os.Exit(1)
		}
		defer fGet.Close()
		getWriters[eng] = bufio.NewWriter(fGet)

		fSet, err := os.Create(filepath.Join(*outputTargets, fmt.Sprintf("targets_%s_set.txt", eng)))
		if err != nil {
			fmt.Printf("Failed to create targets SET file for %s: %v\n", eng, err)
			os.Exit(1)
		}
		defer fSet.Close()
		setWriters[eng] = bufio.NewWriter(fSet)
	}

	keysWriter := bufio.NewWriter(keysFile)

	for i := 0; i < *count; i++ {
		id := gen.GenerateULID()
		keyStr := ulid.EncodeULID(id)

		// 1. Write raw key
		_, err := fmt.Fprintf(keysWriter, "%s\n", keyStr)
		if err != nil {
			fmt.Printf("Failed to write key: %v\n", err)
			os.Exit(1)
		}

		// 2. Write vegeta targets for each engine
		for _, eng := range engines {
			_, err = fmt.Fprintf(getWriters[eng], "GET http://127.0.0.1:%s/%s/get?id=%s\n", *port, eng, keyStr)
			if err != nil {
				fmt.Printf("Failed to write GET target: %v\n", err)
				os.Exit(1)
			}
			_, err = fmt.Fprintf(setWriters[eng], "POST http://127.0.0.1:%s/%s/set?id=%s\n", *port, eng, keyStr)
			if err != nil {
				fmt.Printf("Failed to write SET target: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if err := keysWriter.Flush(); err != nil {
		fmt.Printf("Failed to flush keys writer: %v\n", err)
		os.Exit(1)
	}
	for _, w := range getWriters {
		if err := w.Flush(); err != nil {
			fmt.Printf("Failed to flush GET writer: %v\n", err)
			os.Exit(1)
		}
	}
	for _, w := range setWriters {
		if err := w.Flush(); err != nil {
			fmt.Printf("Failed to flush SET writer: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Successfully generated %d keys in %s and Vegeta target files in %s\n", *count, *outputKeys, *outputTargets)
}
