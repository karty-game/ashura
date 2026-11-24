//go:build !js

package levelbuild

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func productionNativeLibrary(ctx *assetbuild.Context) (string, error) {
	explicit := ctx.Getenv("WGPU_NATIVE_PATH")

	library, err := bake.NativeLibrary()
	if err == nil {
		return library, nil
	}

	if explicit != "" || !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	spec, err := nativeLibrarySpecification(runtime.GOOS + "-" + runtime.GOARCH)
	if err != nil {
		return "", err
	}

	return ctx.ManagedLibrary(spec)
}

func nativeLibrarySpecification(target string) (assetbuild.ManagedToolSpec, error) {
	var archive, digest, file string

	switch target {
	case "linux-amd64":
		archive, digest, file = "wgpu-linux-x86_64-release.zip", "cf614af80f23c6364a13f6569e7ae4ca7367ebf6062e3d3d6e80e205264636b4", "lib/libwgpu_native.so"
	case "linux-arm64":
		archive, digest, file = "wgpu-linux-aarch64-release.zip", "d763c855a15ab77d43638769514e4cb24b2ab025928160ffbd8b791b1d5a083f", "lib/libwgpu_native.so"
	case "darwin-arm64":
		archive, digest, file = "wgpu-macos-aarch64-release.zip", "e3e3f6144f9b197b8199681e956391631cfc6d1cef057a1e8c5a1c6aedef5054", "lib/libwgpu_native.dylib"
	case "windows-amd64":
		archive, digest, file = "wgpu-windows-x86_64-msvc-release.zip", "6ec736b6e7c7259ee5d13cdd06eac4407d1f38539b2e555cbe7040e6098f9dd9", "lib/wgpu_native.dll"
	default:
		return assetbuild.ManagedToolSpec{}, fmt.Errorf("native lightmap library has no supported package for %s: %w", target, ErrInvalid)
	}

	return assetbuild.ManagedToolSpec{Name: "wgpu-native", Version: "29.0.0.0",
		URL:    "https://github.com/gfx-rs/wgpu-native/releases/download/v29.0.0.0/" + archive,
		SHA256: digest, Format: "zip", Executable: file}, nil
}
