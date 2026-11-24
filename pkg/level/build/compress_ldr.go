//go:build !js

package levelbuild

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
)

func discoverEncodedNativeTextures(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	chunks []assetbuild.Ref,
	options PublishOptions,
	profile NativeTextureOptions,
) error {
	inputs, err := collectNativeBlocks(ctx, chunks)
	if err != nil {
		return err
	}

	encoded, dependencies, err := nativeTextureActions(plan, resource.ID, inputs, profile, options.sharedCodecs)
	if err != nil {
		return err
	}

	rewritten, err := nativeTextureIndexActions(plan, resource.ID, chunks, inputs.pages, encoded, dependencies, profile)
	if err != nil {
		return err
	}

	return plan.Expand(
		assetbuild.Expansion{
			ID:     "level/native-texture-publish/" + resource.ID,
			Inputs: append([]assetbuild.Ref{resource.Ref}, rewritten...),
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				return discoverAlbedoCompression(ctx, child, resource, record, rewritten, options)
			},
		},
	)
}

func nativeTextureActions(
	plan *assetbuild.Plan,
	identity string,
	inputs nativeBlockInputs,
	profile NativeTextureOptions,
	shared *codecActionCache,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	encoded := make(map[string]assetbuild.Ref, len(inputs.files))

	var dependencies []assetbuild.Ref

	for _, key := range sortedAlbedoKeys(inputs.files) {
		var (
			ref assetbuild.Ref
			err error
		)

		switch {
		case inputs.lightmaps[key]:
			ref, err = addLDRLightmapAction(plan, identity, key, inputs.files[key], profile, shared)
		case profile.Family == nativeTextureETC2:
			ref, err = addLDRRawSSBumpAction(plan, identity, key, inputs.files[key], shared)
		default:
			encoder := ssbumpEncoderOptions{Encoder: ssbumpEncoder(nativeTextureAlbedoProfile(profile))}
			ref, err = addLDRSSBumpAction(plan, identity, key, inputs.files[key], encoder, texture.BC7, shared)
		}

		if err != nil {
			return nil, nil, err
		}

		encoded[key] = ref
		dependencies = append(dependencies, ref)
	}

	return encoded, dependencies, nil
}

type opaqueLDRPrepared struct {
	Data assetbuild.Artifact `json:"data"`
}
type ldrEncoderOptions struct {
	Encoder assetbuild.BlockCompressionOptions `json:"encoder"`
}

func addLDRLightmapAction(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	profile NativeTextureOptions,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	encoder := ldrEncoderOptions{Encoder: nativeLightmapEncoder(profile)}

	prepared, err := shared.add(plan, assetbuild.Action{
		Parallel:  true,
		ID:        "level/ldr-uastc/" + identity + "/" + key,
		Processor: "level-ldr-uastc",
		Revision:  "opaque-normalized-transfer@1-wasi1",
		Artifacts: []assetbuild.Artifact{file.Artifact},
		Options:   encoder,
		Outputs: []assetbuild.Slot{
			{Name: slot, Schema: "karty.opaque-ldr-uastc@1"},
		},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return prepareOpaqueLDR(ctx, file, encoder.Encoder)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	target := nativeTextureTarget(profile)

	outputs, err := shared.add(plan, assetbuild.Action{
		Parallel:  true,
		ID:        "level/ldr-native/" + identity + "/" + key,
		Processor: nativeBlockProcessor,
		Revision:  "opaque-ldr-native@1-wasi1",
		Inputs:    prepared,
		Options: struct {
			Target  texture.Format
			Encoder ldrEncoderOptions
		}{target, encoder},
		Outputs: []assetbuild.Slot{{Name: slot, Schema: nativeBlockFileSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return transcodeOpaqueLDR(ctx, prepared[0], target, encoder.Encoder)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return outputs[0], nil
}

func prepareOpaqueLDR(ctx *assetbuild.Context, file fileRecord, encoder assetbuild.BlockCompressionOptions) (assetbuild.Result, error) {
	source, err := ctx.ReadArtifact(file.Artifact)
	if err != nil {
		return nil, err
	}

	data, err := assetbuild.PrepareOpaqueLDRUASTC(ctx, source, encoder)
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(data)
	if err != nil {
		return nil, err
	}

	return result(opaqueLDRPrepared{Data: artifact})
}

func transcodeOpaqueLDR(
	ctx *assetbuild.Context,
	source assetbuild.Ref,
	target texture.Format,
	encoder assetbuild.BlockCompressionOptions,
) (assetbuild.Result, error) {
	return transcodeOpaquePrepared(ctx, source, target, encoder, "lightmaps")
}

func transcodeOpaquePrepared(
	ctx *assetbuild.Context,
	source assetbuild.Ref,
	target texture.Format,
	encoder assetbuild.BlockCompressionOptions,
	directory string,
) (assetbuild.Result, error) {
	var prepared opaqueLDRPrepared
	if err := readRecord(ctx, source, &prepared); err != nil {
		return nil, err
	}

	data, err := ctx.ReadArtifact(prepared.Data)
	if err != nil {
		return nil, err
	}

	native, err := assetbuild.TranscodeLDRNative(ctx, data, target, encoder)
	if err != nil {
		return nil, err
	}

	file, err := storePublished(ctx, directory, ".ktx2", native)
	if err != nil {
		return nil, err
	}

	return result(file)
}

func addLDRSSBumpAction(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	encoder ssbumpEncoderOptions,
	target texture.Format,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	prepared, err := addPreparedSSBumpWithExecution(plan, identity, key, file, encoder, shared, true)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	outputs, err := shared.add(plan, assetbuild.Action{
		Parallel:  true,
		ID:        "level/native-ssbump/" + identity + "/" + key,
		Processor: nativeBlockProcessor,
		Revision:  "prepared-opaque-ssbump-native@2-wasi1",
		Inputs:    prepared,
		Options: struct {
			Target  texture.Format       `json:"target"`
			Encoder ssbumpEncoderOptions `json:"encoder"`
		}{target, encoder},
		Outputs: []assetbuild.Slot{
			{Name: slot, Schema: nativeBlockFileSchema},
		},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return transcodeOpaquePrepared(ctx, prepared[0], target, encoder.Encoder, "textures")
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return outputs[0], nil
}
