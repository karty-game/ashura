//go:build !js

package levelbuild

import "github.com/karty-game/ashura/pkg/assetbuild"

// The portable profile retains authored numeric coefficients exactly. Only the
// offline KTX container layout is canonicalized; no block codec or mip filter runs.
func addLDRRawSSBumpAction(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	outputs, err := shared.add(plan, assetbuild.Action{
		Parallel:  true,
		ID:        "level/native-ssbump-rgba8/" + identity + "/" + key,
		Processor: "level-native-ssbump-rgba8",
		Revision:  "linear-authored-mips@1-wasi1",
		Artifacts: []assetbuild.Artifact{file.Artifact},
		Outputs:   []assetbuild.Slot{{Name: slot, Schema: nativeBlockFileSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return prepareNativeSSBumpRGBA8(ctx, file)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return outputs[0], nil
}

func prepareNativeSSBumpRGBA8(ctx *assetbuild.Context, file fileRecord) (assetbuild.Result, error) {
	source, err := ctx.ReadArtifact(file.Artifact)
	if err != nil {
		return nil, err
	}

	data, err := assetbuild.PrepareNativeSSBumpRGBA8(source)
	if err != nil {
		return nil, err
	}

	output, err := storePublished(ctx, "textures", ".ktx2", data)
	if err != nil {
		return nil, err
	}

	return result(output)
}
