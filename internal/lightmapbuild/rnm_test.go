//go:build !js

package lightmapbuild

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	modelbuild "github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestReceiverFrameFollowsMaterialUVAndMirroredHandedness(t *testing.T) {
	t.Parallel()

	positions := [3]vec{{0, 0, 0}, {1, 0, 0}, {0, 0, 1}}

	for _, test := range []struct {
		name    string
		uv      [3][2]float32
		tangent vec
		hand    float32
	}{
		{"ordinary", [3][2]float32{{0, 0}, {1, 0}, {0, 1}}, vec{1, 0, 0}, -1},
		{"mirrored", [3][2]float32{{0, 0}, {-1, 0}, {0, 1}}, vec{-1, 0, 0}, 1},
		{"rotated", [3][2]float32{{0, 0}, {0, 1}, {1, 0}}, vec{0, 0, 1}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tangent, bitangent := triangleTangents(positions, test.uv)
			frame := make([]byte, 16)
			setFrame(frame, 0, vec{0, 1, 0}, tangent, bitangent)

			for axis := range 3 {
				if math.Abs(float64(readFloat(frame[axis*4:])-test.tangent[axis])) > 1e-6 {
					t.Fatal("UV0 orientation changed")
				}
			}

			if readFloat(frame[12:]) != test.hand {
				t.Fatal("mirrored UV handedness changed")
			}
		})
	}
	// Smooth normals need an orthogonalized tangent, including degenerate UVs.
	for _, normal := range []vec{normalize(vec{1, 1, 0}), {0, 1, 0}, {0, 0, 1}} {
		frame := make([]byte, 16)
		setFrame(frame, 0, normal, vec{}, vec{})

		tangent := vec{readFloat(frame), readFloat(frame[4:]), readFloat(frame[8:])}
		if math.Abs(float64(dot(normal, tangent))) > 1e-6 || math.Abs(float64(lengthSquared(tangent)-1)) > 1e-6 {
			t.Fatal("degenerate UV fallback is not orthonormal")
		}
	}
}

func TestReflectanceAveragesLinearTexelsAndTracksBVHOrder(t *testing.T) {
	t.Parallel()

	color := averageReflectance(texturedata.Data{ColorSpace: texturedata.SRGB, Levels: [][]byte{{0, 0, 0, 255, 255, 255, 255, 255}}})
	if color != (vec{.5, .5, .5}) {
		t.Fatalf("averaged sRGB values rather than linear light: %v", color)
	}

	middle := averageReflectance(texturedata.Data{ColorSpace: texturedata.SRGB, Levels: [][]byte{{128, 128, 128, 255}}})
	if math.Abs(float64(middle[0]-.2158605)) > 1e-6 {
		t.Fatal("missing sRGB transfer")
	}

	document := modeldata.Document{
		Parts: []modeldata.Part{
			{Material: modeldata.Material{Albedo: -1, Color: [4]float32{1, .25, .1, 1}}},
			{Material: modeldata.Material{Albedo: -1, Color: [4]float32{.2, .4, .6, 1}, DoubleSided: true}},
		},
	}

	triangles := make([]triangle, 12)
	for index := range triangles {
		x := float32(11 - index)
		triangles[index] = triangle{
			points:  [3]vec{{x, 0, 0}, {x + 1, 0, 0}, {x, 1, 0}},
			center:  vec{x, 0, 0},
			ordinal: index,
			part:    index % 2,
		}
	}

	_, _ = buildBVH(triangles)

	surfaces, err := buildSurfaces(triangles, document)
	if err != nil {
		t.Fatal(err)
	}

	for index, triangle := range triangles {
		expected := float32(.95)
		sided := float32(0)

		if triangle.part == 1 {
			expected = .2
			sided = 1
		}

		if readFloat(surfaces[index*16:]) != expected || readFloat(surfaces[index*16+12:]) != sided {
			t.Fatal("material metadata detached from BVH triangle during sorting")
		}
	}
}

func TestRNMPreparationKeepsGeometryAndBuildsFrames(t *testing.T) {
	t.Parallel()

	input, err := modelbuild.Compile(testfixture.Scene(), "fixture.glb")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := Prepare(ctx, input.Data, Options{Resolution: 128, Padding: 3, RNM: true})
	if err != nil {
		t.Fatal(err)
	}

	document, err := lightmapdata.Decode(result.Lightmap)
	if err != nil {
		t.Fatal(err)
	}

	if !document.RNM() || len(document.Frames) != result.Width*result.Height*16 || len(document.Surfaces) != len(document.Triangles)/48*16 {
		t.Fatal("missing RNM preparation")
	}
}
