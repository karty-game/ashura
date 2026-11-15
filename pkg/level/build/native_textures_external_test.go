//go:build !js

package levelbuild_test

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	levelbuild "github.com/karty-game/karty-webgpu/pkg/level/build"
)

func TestPublicNativeTexturePublisherContract(t *testing.T) {
	t.Parallel()

	for _, family := range []string{"bc", "etc2"} {
		options := levelbuild.PublishOptions{
			NativeTextures: &levelbuild.NativeTextureOptions{
				Family:   family,
				Transfer: "srgb",
				Encoder:  assetbuild.BlockCompressionOptions{},
			},
		}
		if err := levelbuild.Publish(options).Plan(&assetbuild.Plan{}); err != nil {
			t.Fatal("LDR profile requires unrelated HDR tools", family, err)
		}

		options.NativeHDR = &levelbuild.NativeHDROptions{}
		if err := levelbuild.Publish(options).Plan(&assetbuild.Plan{}); !errors.Is(err, levelbuild.ErrInvalid) {
			t.Fatal("conflicting native profiles accepted", err)
		}
	}
}
