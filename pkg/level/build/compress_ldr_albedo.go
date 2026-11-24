//go:build !js

package levelbuild

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
)

// The V7 path retains the same shared ETC1S producer, then writes externally
// valid native mip containers directly. Legacy profile bytes remain unchanged.
func nativeLDRAlbedoActions(
	plan *assetbuild.Plan,
	identity string,
	files map[string]fileRecord,
	profile NativeHDROptions,
	shared *codecActionCache,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	return planNativeAlbedoActions(plan, identity, files, profile, shared, "opaque-native-srgb-mips@2", encodeLDRAlbedo, true)
}

func encodeLDRAlbedo(ctx *assetbuild.Context, file fileRecord, profile NativeHDROptions) (assetbuild.Result, error) {
	source, err := ctx.ReadArtifact(file.Artifact)
	if err != nil {
		return nil, err
	}

	target := texture.BC1
	if profile.Family == nativeHDRMobileFamily {
		target = texture.ETC2RGB
	}

	data, err := assetbuild.TranscodeLDRNative(ctx, source, target, profile.Encoder)
	if err != nil {
		return nil, err
	}

	output, err := storePublished(ctx, "textures", ".ktx2", data)
	if err != nil {
		return nil, err
	}

	return result(output)
}
