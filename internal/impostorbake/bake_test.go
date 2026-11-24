//go:build !js

package impostorbake

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestBakeDeterministicPreparedQuadAndViewCapture(t *testing.T) {
	t.Parallel()

	source := fixtureDocument()
	options := Options{Resolution: 32, Azimuths: 4, Elevations: 3, Padding: 2, MipLevels: 2}

	first, err := Bake(t.Context(), source, options)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Bake(t.Context(), source, options)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := impostordata.Encode(first)
	if err != nil {
		t.Fatal(err)
	}

	repeated, err := impostordata.Encode(second)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encoded, repeated) {
		t.Fatal("identical captures changed container bytes")
	}

	decoded, err := impostordata.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.SourceTriangles != 2 || len(decoded.Views) != 12 || decoded.Center != [3]float32{} || decoded.Roughness != .7 ||
		len(decoded.Vertices) != 4*meshdata.VertexStride ||
		len(decoded.Indices) != 6*meshdata.IndexStride ||
		len(decoded.Uniform) != impostordata.UniformSize {
		t.Fatalf("prepared descriptor lost source/quad facts: %+v", decoded)
	}

	if _, err := meshdata.FromOwned(decoded.Vertices, decoded.Indices); err != nil {
		t.Fatal(err)
	}

	view := decoded.Views[4]
	if math.Abs(float64(view.Outward[2]-1)) > 1e-6 || math.Abs(float64(view.Right[0]-1)) > 1e-6 || math.Abs(float64(view.Up[1]-1)) > 1e-6 {
		t.Fatalf("front capture orientation %+v", view)
	}

	color, depth := centerTexel(t, decoded.Color, decoded), centerTexel(t, decoded.NormalDepth, decoded)
	if color[0] < 250 || color[1] > 2 || color[2] > 2 || color[3] != 255 || depth[2] < 250 || depth[3] <= 128 {
		t.Fatalf("front-most red surface/normal/depth lost: color=%v normalDepth=%v", color, depth)
	}
}

func TestBakePreservesLinearTintAndTangentNormalMap(t *testing.T) {
	t.Parallel()

	source := fixtureDocument()
	source.Parts = source.Parts[:1]
	source.Parts[0].Material.Color = [4]float32{.5, 1, 1, 1}
	source.Parts[0].Material.Albedo = 0
	source.Parts[0].Material.Normal = 1

	albedo, err := assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:   "Albedo",
			Source: "albedo.png",
			Usage:  assetbuild.UsageAlbedo,
			Width:  1,
			Height: 1,
			Levels: [][]byte{{128, 128, 128, 255}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	normal, err := assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:   "Normal",
			Source: "normal.png",
			Usage:  assetbuild.UsageNormal,
			Width:  1,
			Height: 1,
			Levels: [][]byte{{255, 128, 128, 255}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	source.Textures = []modeldata.Texture{{Role: modeldata.Albedo, Data: albedo}, {Role: modeldata.Normal, Data: normal}}

	baked, err := Bake(t.Context(), source, Options{Resolution: 32, Azimuths: 4, Elevations: 3, Padding: 2, MipLevels: 2})
	if err != nil {
		t.Fatal(err)
	}

	color, depth := centerTexel(t, baked.Color, baked), centerTexel(t, baked.NormalDepth, baked)
	if color[0] < 90 || color[0] > 94 || color[1] < 127 || color[1] > 129 || depth[0] < 250 || depth[2] < 126 || depth[2] > 131 {
		t.Fatalf("linear tint or tangent-space normal not preserved: color=%v normalDepth=%v", color, depth)
	}
}

func TestBakeRejectsMaterialLossAndInvalidCaptureSettings(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		modify func(*modeldata.Document)
	}{
		{"roughness", func(value *modeldata.Document) { value.Parts[1].Material.Roughness = .2 }},
		{"ao-map", func(value *modeldata.Document) { value.Parts[0].Material.AO = 0 }},
		{"out-of-range-normal", func(value *modeldata.Document) { value.Parts[0].Material.Normal = 8 }},
		{"non-finite", func(value *modeldata.Document) {
			binary.LittleEndian.PutUint32(value.Parts[0].Vertices, math.Float32bits(float32(math.Inf(1))))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := fixtureDocument()
			test.modify(&source)

			if _, err := Bake(t.Context(), source, Options{}); err == nil {
				t.Fatal("silently discarded unsupported source data")
			}
		})
	}

	for _, options := range []Options{{Resolution: 17}, {Azimuths: 64, Elevations: 64}, {Padding: 1, MipLevels: 3}, {MipLevels: 100}, {ElevationMin: 20, ElevationMax: -20}, {Resolution: 512, Azimuths: 16}} {
		if _, err := Defaults(options); err == nil {
			t.Fatalf("accepted invalid capture settings %+v", options)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Bake(ctx, fixtureDocument(), Options{}); err == nil {
		t.Fatal("ignored canceled capture")
	}
}

func TestIndependentCellMipsNeverBlendAdjacentViews(t *testing.T) {
	t.Parallel()

	options := Options{Resolution: 16, Azimuths: 2, Elevations: 2, Padding: 4, MipLevels: 3}
	colors, normals := atlasLevels(options)

	for elevation := range 2 {
		for azimuth := range 2 {
			colorPixels, normalPixels := make([]byte, 16*16*4), make([]byte, 16*16*4)
			for pixel := range 16 * 16 {
				colorPixels[pixel*4+azimuth], colorPixels[pixel*4+3] = 255, 255
				normalPixels[pixel*4+2], normalPixels[pixel*4+3] = 255, 64
			}

			packView(colors, normals, colorPixels, normalPixels, azimuth, elevation, options)
		}
	}

	for level, data := range colors {
		cell := 24 >> level
		width := cell * 2

		left, right := ((cell/2)*width+cell-1)*4, ((cell/2)*width+cell)*4
		if data[left] != 255 || data[left+1] != 0 || data[right] != 0 || data[right+1] != 255 {
			t.Fatalf("view cells bled at mip%d: left%v right%v", level, data[left:left+4], data[right:right+4])
		}
	}
}

func TestTransparentMipFilteringDoesNotDarkenOrInventCoverage(t *testing.T) {
	t.Parallel()

	colors := []byte{255, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	normals := []byte{128, 128, 255, 200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	color, normal := reduce(colors, normals, 2)
	if color[0] != 255 || color[1] != 0 || color[3] != 64 || normal[2] < 250 || normal[3] != 200 {
		t.Fatalf("coverage-weighted mip color=%v normalDepth=%v", color, normal)
	}

	baseAlpha := colors[7]
	dilate(colors, normals, 2)

	if colors[4] != 255 || colors[7] != baseAlpha || normals[7] != 200 {
		t.Fatalf("dilation changed coverage or lost foreground depth: %v %v", colors, normals)
	}
}

func fixtureDocument() modeldata.Document {
	return modeldata.Document{Source: "assets/prefab.glb", Parts: []modeldata.Part{
		fixturePart(.25, [4]float32{1, 0, 0, 1}), fixturePart(-.25, [4]float32{0, 0, 1, 1}),
	}}
}

func fixturePart(depth float32, color [4]float32) modeldata.Part {
	var vertices, indices []byte

	for _, value := range [][8]float32{{-.5, -.5, depth, 0, 0, 1, 0, 0}, {.5, -.5, depth, 0, 0, 1, 1, 0}, {0, .5, depth, 0, 0, 1, .5, 1}} {
		for _, channel := range value {
			vertices = binary.LittleEndian.AppendUint32(vertices, math.Float32bits(channel))
		}
	}

	for _, index := range []uint32{0, 1, 2} {
		indices = binary.LittleEndian.AppendUint32(indices, index)
	}

	return modeldata.Part{
		Name:     "part",
		Vertices: vertices,
		Indices:  indices,
		Material: modeldata.Material{Color: color, Roughness: .7, NormalStrength: 1, Albedo: -1, Normal: -1, AO: -1},
	}
}

func centerTexel(t *testing.T, data []byte, document impostordata.Document) [4]byte {
	t.Helper()

	return viewTexel(t, data, document, 4)
}
