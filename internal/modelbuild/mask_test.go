//go:build !js

package modelbuild_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
)

func maskSource(t *testing.T) (map[string]any, []byte) {
	t.Helper()

	document, data := testfixture.Triangle()
	data = testfixture.EmbedTexture(document, data)
	views := maskFixtureValue[[]any](t, document["bufferViews"])
	view := maskFixtureValue[map[string]any](t, views[len(views)-1])

	const dimension = 2

	image := image.NewNRGBA(image.Rect(0, 0, dimension, dimension))
	for index, alpha := range []byte{0, 64, 128, 255} {
		image.SetNRGBA(index%dimension, index/dimension, color.NRGBA{R: 80, G: 160, B: 240, A: alpha})
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image); err != nil {
		t.Fatal(err)
	}

	data = append(data[:maskFixtureValue[int](t, view["byteOffset"])], encoded.Bytes()...)
	view["byteLength"] = encoded.Len()
	maskFixtureValue[map[string]any](t, maskFixtureValue[[]any](t, document["buffers"])[0])["byteLength"] = len(data)

	return document, data
}

func TestMaskedGLBPreservesTintCoverageSharedImageAndOfflineMips(t *testing.T) {
	t.Parallel()

	document, data := maskSource(t)
	opaque := maskFixtureValue[map[string]any](t, maskFixtureValue[[]any](t, document["materials"])[0])
	maskFixtureValue[map[string]any](t, opaque["pbrMetallicRoughness"])["baseColorFactor"] = []float64{1, .7, .25, .25}
	masked := map[string]any{
		"alphaMode": "MASK", "doubleSided": true,
		"pbrMetallicRoughness": map[string]any{
			"metallicFactor": 0, "baseColorFactor": []float64{1, .7, .25, .75},
			"baseColorTexture": map[string]any{"index": 0},
		},
	}
	document["materials"] = append(maskFixtureValue[[]any](t, document["materials"]), masked)
	mesh := maskFixtureValue[map[string]any](t, maskFixtureValue[[]any](t, document["meshes"])[0])
	primitive := maskFixtureValue[map[string]any](t, maskFixtureValue[[]any](t, mesh["primitives"])[0])
	mesh["primitives"] = append(maskFixtureValue[[]any](t, mesh["primitives"]), map[string]any{
		"attributes": primitive["attributes"], "indices": primitive["indices"], "material": 1,
	})

	result, err := modelbuild.Compile(testfixture.GLB(document, data), "masked-shared.glb")
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := modeldata.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}

	const wireV2 = 2
	if prepared.Source != "masked-shared.glb" || binary.LittleEndian.Uint32(result.Data[8:]) != wireV2 || len(prepared.Textures) != 1 {
		t.Fatal("source provenance or immutable shared image changed", prepared)
	}

	first, second := prepared.Parts[0].Material, prepared.Parts[1].Material
	if first.AlphaMask || first.AlphaCutoff != 0 || first.Color[3] != 1 || !second.AlphaMask ||
		second.AlphaCutoff != .5 || second.Color[3] != .75 || !second.DoubleSided || first.Albedo != second.Albedo {
		t.Fatal("coverage/material binding changed", first, second)
	}

	image, err := ktx2.Decode(prepared.Textures[0].Data)
	if err != nil || image.ColorSpace != texture.SRGB || len(image.Levels) != 2 {
		t.Fatal("masked texture is not GPU-ready sRGB with offline mips", err)
	}

	const channels = 4
	for index, alpha := range []byte{0, 64, 128, 255} {
		if image.Levels[0][index*channels+channels-1] != alpha {
			t.Fatal("authored source alpha was lost")
		}
	}

	if image.Levels[1][channels-1] != 112 {
		t.Fatal("authored alpha did not reach the prepared mip")
	}
}

func TestMaskedGLBCutoffBoundsAndExplicitEndpoints(t *testing.T) {
	t.Parallel()

	for _, cutoff := range []float64{-1, 0, .35, 1, 2} {
		document, data := testfixture.Triangle()
		material := maskFixtureValue[map[string]any](t, maskFixtureValue[[]any](t, document["materials"])[0])
		material["alphaMode"], material["alphaCutoff"] = "MASK", cutoff

		result, err := modelbuild.Compile(testfixture.GLB(document, data), "cutoff.glb")
		if cutoff < 0 || cutoff > 1 {
			if err == nil {
				t.Fatal("out-of-range cutoff admitted", cutoff)
			}

			continue
		}

		if err != nil {
			t.Fatal("valid cutoff rejected", cutoff, err)
		}

		prepared, err := modeldata.Decode(result.Data)
		if err != nil || float64(prepared.Parts[0].Material.AlphaCutoff) != float64(float32(cutoff)) {
			t.Fatal("explicit cutoff changed", cutoff, err)
		}
	}
}

func maskFixtureValue[T any](t *testing.T, value any) T {
	t.Helper()

	converted, ok := value.(T)
	if !ok {
		t.Fatalf("unexpected cutout fixture type %T", value)
	}

	return converted
}
