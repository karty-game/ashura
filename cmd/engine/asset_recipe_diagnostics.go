//go:build !js

package main

const assetRecipeStderrLimit = 8 * 1024

// Asset recipes stream stderr live, while gallery stdout is grouped by project.
// Retain a bounded tail so the returned failure repeats the cause after those
// buffered progress logs. os/exec serializes writes to this stderr destination.
type assetRecipeStderr struct {
	data [assetRecipeStderrLimit]byte
	used int
}

func (diagnostics *assetRecipeStderr) Write(input []byte) (int, error) {
	length := len(input)
	if len(input) > len(diagnostics.data) {
		input = input[len(input)-len(diagnostics.data):]
	}

	if overflow := diagnostics.used + len(input) - len(diagnostics.data); overflow > 0 {
		copy(diagnostics.data[:], diagnostics.data[overflow:diagnostics.used])
		diagnostics.used -= overflow
	}

	diagnostics.used += copy(diagnostics.data[diagnostics.used:], input)

	return length, nil
}

func (diagnostics *assetRecipeStderr) String() string {
	return string(diagnostics.data[:diagnostics.used])
}
