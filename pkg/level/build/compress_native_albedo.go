//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func discoverNativeAlbedos(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	chunks []assetbuild.Ref,
	options PublishOptions,
) error {
	profile, enabled := nativeAlbedoProfile(options)
	if !enabled {
		return publishLevel(ctx, plan, resource, record, chunks, options)
	}

	if options.AlbedoETC1S == nil {
		return fmt.Errorf("native albedos require offline ETC1S preparation: %w", ErrInvalid)
	}

	pages, files, err := collectAlbedos(ctx, chunks)
	if err != nil {
		return err
	}

	planner := nativeAlbedoActions
	if options.NativeTextures != nil {
		planner = nativeLDRAlbedoActions
	}

	encoded, dependencies, err := planner(plan, resource.ID, files, profile, options.sharedCodecs)
	if err != nil {
		return err
	}

	rewritten, err := nativeAlbedoIndexActions(plan, resource.ID, chunks, pages, encoded, dependencies, profile.Family)
	if err != nil {
		return err
	}

	return plan.Expand(
		assetbuild.Expansion{
			ID:     "level/native-albedo-publish/" + resource.ID,
			Inputs: append([]assetbuild.Ref{resource.Ref}, rewritten...),
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				return publishLevel(ctx, child, resource, record, rewritten, options)
			},
		},
	)
}

func nativeAlbedoActions(
	plan *assetbuild.Plan,
	identity string,
	files map[string]fileRecord,
	profile NativeHDROptions,
	shared *codecActionCache,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	return planNativeAlbedoActions(plan, identity, files, profile, shared, "native-srgb-mips@1", encodeNativeAlbedo, false)
}

func planNativeAlbedoActions(
	plan *assetbuild.Plan,
	identity string,
	files map[string]fileRecord,
	profile NativeHDROptions,
	shared *codecActionCache,
	revision string,
	encode func(*assetbuild.Context, fileRecord, NativeHDROptions) (assetbuild.Result, error),
	parallel bool,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	encoded := map[string]assetbuild.Ref{}

	var dependencies []assetbuild.Ref

	for _, key := range sortedAlbedoKeys(files) {
		file := files[key]

		refs, err := shared.add(plan, assetbuild.Action{
			Parallel:  parallel,
			ID:        "level/native-albedo/" + identity + "/" + key,
			Processor: "level-native-albedo",
			Revision:  revision + "-wasi1",
			Artifacts: []assetbuild.Artifact{file.Artifact},
			Options:   profile,
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.native-albedo-file@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return encode(ctx, file, profile)
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

func encodeNativeAlbedo(ctx *assetbuild.Context, file fileRecord, profile NativeHDROptions) (assetbuild.Result, error) {
	encoder := profile.Encoder

	source, err := ctx.ReadArtifact(file.Artifact)
	if err != nil {
		return nil, err
	}

	target := texturedata.BC1
	if profile.Family == "etc2-astc" {
		target = texturedata.ETC2RGB
	}

	data, err := assetbuild.TranscodeAlbedoNative(ctx, source, target, encoder)
	if err != nil {
		return nil, err
	}

	output, err := storePublished(ctx, "textures", ".ktx2", data)
	if err != nil {
		return nil, err
	}

	return result(output)
}

func nativeAlbedoIndexActions(
	plan *assetbuild.Plan,
	identity string,
	chunks []assetbuild.Ref,
	pages [][]publishedPage,
	encoded map[string]assetbuild.Ref,
	dependencies []assetbuild.Ref,
	family string,
) ([]assetbuild.Ref, error) {
	var rewritten []assetbuild.Ref

	encoding := "bc1"
	if family == "etc2-astc" {
		encoding = "etc2"
	}

	for index, source := range chunks {
		current := pages[index]

		refs, err := plan.Add(assetbuild.Action{
			ID: fmt.Sprintf(
				"level/native-albedo-index/%s/%d",
				identity,
				index,
			),
			Processor: "level-native-albedo-index",
			Revision:  "replace-native-srgb@1-wasi1",
			Options:   encoding,
			Inputs:    append([]assetbuild.Ref{source}, dependencies...),
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return rewriteAlbedoEncoding(ctx, current, encoded, encoding)
			},
		})
		if err != nil {
			return nil, err
		}

		rewritten = append(rewritten, refs[0])
	}

	return rewritten, nil
}

func nativeAlbedoProfile(options PublishOptions) (NativeHDROptions, bool) {
	if options.NativeTextures != nil {
		profile := nativeTextureAlbedoProfile(*options.NativeTextures)

		return profile, profile.NativeAlbedo
	}

	if options.NativeHDR != nil {
		return *options.NativeHDR, options.NativeHDR.NativeAlbedo
	}

	return NativeHDROptions{}, false
}
