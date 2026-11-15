//go:build !js

package modelbuild

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Prepare compiles an explicit source GLB and atomically publishes its prepared
// bytes only after the entire model succeeds. Identical output is not rewritten.
func Prepare(input, output string) (Result, error) {
	source, err := filepath.Abs(input)
	if err != nil {
		return Result{}, err
	}

	destination, err := filepath.Abs(output)
	if err != nil || source == destination {
		return Result{}, errFormat
	}

	if err := realParents(source); err != nil {
		return Result{}, err
	}

	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("source must be a regular GLB file: %w", errFormat)
	}

	if existing, statErr := os.Stat(destination); statErr == nil && os.SameFile(info, existing) {
		return Result{}, errFormat
	}

	//nolint:gosec // Explicit CLI source path, regular-file and real-parent validation precedes access.
	file, err := os.Open(source)
	if err != nil {
		return Result{}, fmt.Errorf("open GLB: %w", err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxSourceBytes+1))
	if err != nil {
		return Result{}, err
	}

	result, err := Compile(data, filepath.ToSlash(input))
	if err != nil {
		return Result{}, err
	}

	if err := writePrepared(destination, result.Data); err != nil {
		return Result{}, err
	}

	return result, nil
}

func writePrepared(path string, data []byte) error {
	if err := realParents(path); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	if err == nil && !info.Mode().IsRegular() || err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("prepared output must be a regular file: %w", errFormat)
	}

	if err == nil && info.Size() == int64(len(data)) && identicalFile(path, data) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("prepared directory: %w", err)
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".model-*.kmodel")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()

	if _, err := file.Write(data); err != nil {
		_ = file.Close()

		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(file.Name(), path)
}

func realParents(path string) error {
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		info, err := os.Lstat(directory)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("model path parent must be a real directory: %w", errFormat)
		}

		if filepath.Dir(directory) == directory {
			return nil
		}
	}
}

func identicalFile(path string, data []byte) bool {
	//nolint:gosec // Explicit regular CLI output under already validated real parents; read is bounded.
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	existing, err := io.ReadAll(io.LimitReader(file, int64(len(data))+1))

	return err == nil && bytes.Equal(existing, data)
}
