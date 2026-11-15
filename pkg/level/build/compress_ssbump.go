//go:build !js

package levelbuild

import (
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type ssbumpPrepared struct {
	Data assetbuild.Artifact `json:"data"`
}

type ssbumpEncoderOptions struct {
	Encoder assetbuild.BlockCompressionOptions `json:"encoder"`
}

// ssbumpEncoder retains only fields consumed by the linear UASTC/native KTX
// stages. BC6H effort cannot affect these coefficient bytes.
func ssbumpEncoder(profile NativeHDROptions) assetbuild.BlockCompressionOptions {
	return assetbuild.BlockCompressionOptions{
		Timeout:       profile.Encoder.Timeout,
		SSBumpQuality: profile.Encoder.SSBumpQuality,
	}
}

func addSSBumpNativeAction(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	profile NativeHDROptions,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	encoder := ssbumpEncoderOptions{Encoder: ssbumpEncoder(profile)}

	target := texture.BC7
	if nativeHDRFamily(profile) == nativeHDRMobileFamily {
		target = texture.ASTC4x4
	}

	return addSSBumpTargetAction(plan, identity, key, file, encoder, target, shared)
}

func addSSBumpTargetAction(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	encoder ssbumpEncoderOptions,
	target texture.Format,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	prepared, err := addPreparedSSBump(plan, identity, key, file, encoder, shared)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	refs, err := shared.add(plan, assetbuild.Action{
		ID:        "level/native-block/" + identity + "/" + key,
		Processor: nativeBlockProcessor, Revision: "prepared-ssbump-native@1-wasi1",
		Inputs: prepared,
		Options: struct {
			Target  texture.Format       `json:"target"`
			Encoder ssbumpEncoderOptions `json:"encoder"`
		}{target, encoder},
		Outputs: []assetbuild.Slot{{Name: slot, Schema: nativeBlockFileSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return transcodePreparedSSBump(ctx, prepared[0], target, encoder.Encoder)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func addPreparedSSBump(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	options ssbumpEncoderOptions,
	shared *codecActionCache,
) ([]assetbuild.Ref, error) {
	return addPreparedSSBumpWithExecution(plan, identity, key, file, options, shared, false)
}

func addPreparedSSBumpWithExecution(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	options ssbumpEncoderOptions,
	shared *codecActionCache,
	parallel bool,
) ([]assetbuild.Ref, error) {
	return shared.add(plan, assetbuild.Action{
		Parallel:  parallel,
		ID:        "level/ssbump-uastc/" + identity + "/" + key,
		Processor: "level-ssbump-uastc", Revision: "linear-coefficients-uastc-mips@3-wasi1",
		Artifacts: []assetbuild.Artifact{file.Artifact},
		Options:   options,
		Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.ssbump-uastc@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return prepareSSBumpUASTC(ctx, file, options.Encoder)
		},
	})
}

func prepareSSBumpUASTC(ctx *assetbuild.Context, file fileRecord, encoder assetbuild.BlockCompressionOptions) (assetbuild.Result, error) {
	source, err := ctx.ReadArtifact(file.Artifact)
	if err != nil {
		return nil, err
	}

	data, err := assetbuild.PrepareSSBumpUASTC(ctx, source, encoder)
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(data)
	if err != nil {
		return nil, err
	}

	return result(ssbumpPrepared{Data: artifact})
}

func transcodePreparedSSBump(
	ctx *assetbuild.Context,
	source assetbuild.Ref,
	target texture.Format,
	encoder assetbuild.BlockCompressionOptions,
) (assetbuild.Result, error) {
	var input ssbumpPrepared
	if err := readRecord(ctx, source, &input); err != nil {
		return nil, err
	}

	data, err := ctx.ReadArtifact(input.Data)
	if err != nil {
		return nil, err
	}

	data, err = assetbuild.TranscodeSSBumpNative(ctx, data, target, encoder)
	if err != nil {
		return nil, err
	}

	output, err := storePublished(ctx, "textures", ".ktx2", data)
	if err != nil {
		return nil, err
	}

	return result(output)
}
