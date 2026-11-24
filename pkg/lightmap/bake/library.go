//go:build !js

package lightmapbake

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

var errNativeLibrary = errors.New("offline baking requires the rendering binding's wgpu-native library in ./lib or WGPU_NATIVE_PATH")

// NativeLibrary resolves the binding's explicit or local native library to a
// file that an offline build can fingerprint. Configure it before first use of
// WebGPU in a process. System-loader-only installations must set WGPU_NATIVE_PATH
// so cached bakes can track the actual dependency rather than a library name.
func NativeLibrary() (string, error) {
	if selected := wgpu.InitializedLibrary(); selected != "" {
		if !filepath.IsAbs(selected) {
			return "", fmt.Errorf("initialized WebGPU library requires an explicit file path: %w", errNativeLibrary)
		}

		return nativeLibraryFile(selected)
	}

	if selected := os.Getenv("WGPU_NATIVE_PATH"); selected != "" {
		return nativeLibraryFile(selected)
	}

	name := "libwgpu_native.so"

	switch runtime.GOOS {
	case "windows":
		name = "wgpu_native.dll"
	case "darwin":
		name = "libwgpu_native.dylib"
	}

	for _, candidate := range []string{filepath.Join("lib", name), name} {
		if _, err := os.Stat(candidate); err == nil {
			return nativeLibraryFile(candidate)
		}
	}

	return "", errors.Join(errNativeLibrary, os.ErrNotExist)
}
func nativeLibraryFile(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	//nolint:gosec // G703: the explicitly selected native dependency may live outside project source.
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect native WebGPU library: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("native WebGPU library is not a regular file: %w", errNativeLibrary)
	}

	return absolute, nil
}

// InitializeNativeLibrary selects the renderer's shared binding library for an
// offline bake. It does not create a GPU device or change environment variables.
func InitializeNativeLibrary(path string) error { return wgpu.InitWithLibrary(path) }
