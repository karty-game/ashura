//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// Consume the declared ETC1S producer directly. The encoder and complete native
// file admission are identical to the staged path, without an intermediate
// per-profile index whose only purpose was to discover this producer's file.
func nativeVariantAlbedos(
	plan *assetbuild.Plan,
	state nativeVariantState,
	prepared map[string]assetbuild.Ref,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	encoded := make(map[string]assetbuild.Ref, len(prepared))

	var dependencies []assetbuild.Ref

	profile := nativeTextureAlbedoProfile(*state.options.NativeTextures)

	for _, key := range sortedAlbedoKeys(prepared) {
		source := prepared[key]

		refs, err := state.options.sharedCodecs.add(plan, assetbuild.Action{
			Parallel: true, ID: "level/native-albedo/" + state.resource.ID + "/" + key,
			Processor: "level-native-albedo", Revision: "opaque-native-srgb-reference@1",
			Inputs:  []assetbuild.Ref{source},
			Options: profile,
			Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.native-albedo-file@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var file fileRecord
				if err := readRecord(ctx, source, &file); err != nil {
					return nil, err
				}

				return encodeLDRAlbedo(ctx, file, profile)
			},
		})
		if err != nil {
			return nil, nil, err
		}

		encoded[key] = refs[0]
		dependencies = append(dependencies, refs[0])
	}

	return encoded, dependencies, nil
}

func planVariantIndices(plan *assetbuild.Plan, state nativeVariantState, codecs nativeVariantCodecs) ([]assetbuild.Ref, error) {
	var rewritten []assetbuild.Ref

	profile := *state.options.NativeTextures

	encoding := "bc1"
	if profile.Family == nativeTextureETC2 {
		encoding = "etc2"
	}

	for index, source := range state.refs {
		pages := codecs.pages[index]

		refs, err := plan.Add(assetbuild.Action{
			Parallel: true, ID: fmt.Sprintf("level/native-profile-index/%s/%d", state.resource.ID, index),
			Processor: "level-native-profile-index", Revision: "native-materials-and-lightmaps@1",
			Inputs:  append([]assetbuild.Ref{source}, codecs.dependencies...),
			Options: struct{ Storage, Transfer, Albedo string }{nativeTextureStorage(profile), profile.Transfer, encoding},
			Outputs: []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				if err := applyNativeTextures(ctx, pages, codecs.textures, profile); err != nil {
					return nil, err
				}

				return rewriteAlbedoEncoding(ctx, pages, codecs.albedos, encoding)
			},
		})
		if err != nil {
			return nil, err
		}

		rewritten = append(rewritten, refs[0])
	}

	return rewritten, nil
}
