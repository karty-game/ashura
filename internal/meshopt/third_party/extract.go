//go:build ignore

// Extract the unchanged upstream WASM bytes from its published JS distribution.
// Usage: go run extract.go meshopt_encoder.js encoder.wasm
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		panic("expected upstream JS and output WASM paths")
	}
	source, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(source)) != "e79ba4a73a758f315ab153dc50c389c7b0bea2b5c1fdcfcaa62ad6d0c27cb25c" {
		panic("unexpected upstream distribution hash")
	}
	packed := regexp.MustCompile(`(?s)var wasm\s*=\s*'([^']*)'`).FindSubmatch(source)
	table := regexp.MustCompile(`(?s)var wasmpack = new Uint8Array\(\[([^]]*)\]`).FindSubmatch(source)
	if len(packed) != 2 || len(table) != 2 {
		panic("unexpected upstream packing")
	}
	var dictionary []byte
	for _, field := range strings.Split(string(table[1]), ",") {
		if strings.TrimSpace(field) == "" {
			continue
		}
		value, err := strconv.ParseUint(strings.TrimSpace(field), 10, 8)
		if err != nil {
			panic(err)
		}
		dictionary = append(dictionary, byte(value))
	}
	if len(dictionary) != 60 {
		panic("unexpected upstream dictionary")
	}
	input := append([]byte(nil), packed[1]...)
	for i, ch := range input {
		switch {
		case ch > 96:
			input[i] = ch - 97
		case ch > 64:
			input[i] = ch - 39
		default:
			input[i] = ch + 4
		}
	}
	var output []byte
	for i := 0; i < len(input); i++ {
		if input[i] < 60 {
			output = append(output, dictionary[input[i]])
		} else {
			if i+1 >= len(input) {
				panic("truncated packing")
			}
			output = append(output, byte(int(input[i]-60)*64+int(input[i+1])))
			i++
		}
	}
	if len(output) < 8 || string(output[:4]) != "\x00asm" {
		panic("invalid WASM")
	}
	if err := os.WriteFile(os.Args[2], output, 0600); err != nil {
		panic(err)
	}
	fmt.Printf("source %x\nwasm %x (%d bytes)\n", sha256.Sum256(source), sha256.Sum256(output), len(output))
}
