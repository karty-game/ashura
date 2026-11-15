//go:build linux

package lightmapbuild_test

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	lightmapbuild "github.com/karty-game/karty-webgpu/pkg/lightmap/build"
)

func TestHardwareRNMVisibilityBasisEnvironmentAndBounce(t *testing.T) {
	t.Parallel()

	if os.Getenv("KARTY_LIGHTMAP_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAP_GPU_TEST=1 with hardware GPU access")
	}

	document := lightmapfixture.RNMDocument()
	binary.LittleEndian.PutUint32(document.Surfaces, math.Float32bits(.95))
	binary.LittleEndian.PutUint32(document.Surfaces[4:], 0)
	binary.LittleEndian.PutUint32(document.Surfaces[8:], 0)

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	bake := func(scene lightmap.RNMScene) lightmapbuild.RNMResult {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := lightmapbuild.BakeRNMBrowser(
			ctx,
			asset,
			scene,
			lightmapbuild.BrowserOptions{Executable: os.Getenv("KARTY_LIGHTMAP_BROWSER")},
		)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf(
			"hardware=%s duration=%s scale=%g samples=%d bounces=%d",
			result.Adapter,
			result.Duration,
			result.Scale,
			result.Samples,
			result.Bounces,
		)

		for _, image := range result.Textures {
			pixels := image.Levels[0]
			for channel := range 4 {
				if pixels[8+channel] != pixels[4+channel] || pixels[12+channel] != 0 {
					t.Fatal("gutter/empty output invalid")
				}
			}
		}

		return result
	}
	point := lightmap.Point{Position: [3]float32{0, 2, 0}, Color: [3]float32{1, .5, 0}, Intensity: 2, Range: 10}

	direct := bake(lightmap.RNMScene{Points: []lightmap.Point{point}})
	for _, image := range direct.Textures {
		if image.Levels[0][0] != 0 || image.Levels[0][1] != 0 || image.Levels[0][2] != 0 {
			t.Fatal("blocked direct light leaked")
		}
	}
	// Align the unobstructed second receiver's light with the first RNM basis.
	point.Position = [3]float32{1 + 2*float32(math.Sqrt(2.0/3)), 2 / float32(math.Sqrt(3)), 0}
	point.Color = [3]float32{1, 1, 1}
	basis := bake(lightmap.RNMScene{Points: []lightmap.Point{point}})

	expected := 255 * .96 * .96
	if math.Abs(float64(basis.Textures[0].Levels[0][4])-expected) > 1 || basis.Textures[1].Levels[0][4] > 1 ||
		basis.Textures[2].Levels[0][4] > 1 {
		t.Fatal("RNM directional projections differ from orthonormal basis")
	}

	sky := bake(lightmap.RNMScene{SkyColor: [3]float32{.5, .5, .5}, GroundColor: [3]float32{.5, .5, .5}, Samples: 64})
	if sky.Textures[0].Levels[0][4] < 30 || sky.Textures[1].Levels[0][4] < 30 || sky.Textures[2].Levels[0][4] < 30 {
		t.Fatal("escaped environment rays failed to illuminate open receiver")
	}

	point.Position = [3]float32{-1, .5, 0}
	point.Intensity = 4
	zero := bake(lightmap.RNMScene{Points: []lightmap.Point{point}, Samples: 64})
	bounce := bake(lightmap.RNMScene{Points: []lightmap.Point{point}, Samples: 64, Bounces: 1})

	var redIncrease, greenIncrease float64

	for i := range 3 {
		a, b := zero.Textures[i].Levels[0], bounce.Textures[i].Levels[0]
		redIncrease += float64(b[0])*float64(bounce.Scale) - float64(a[0])*float64(zero.Scale)
		greenIncrease += float64(b[1])*float64(bounce.Scale) - float64(a[1])*float64(zero.Scale)
	}
	// Each RGBA8 sample can differ from its irradiance by half its encoding
	// step. Comparing three maps encoded with different scales therefore allows
	// 3 * (zero.Scale + bounce.Scale) / 2 byte-times-scale units, plus float
	// arithmetic error. The red gain must exceed that same bound.
	quantizationBound := 1.5*(float64(zero.Scale)+float64(bounce.Scale)) + 1e-4
	if redIncrease <= quantizationBound || math.Abs(greenIncrease) > quantizationBound {
		t.Fatalf("colored one-bounce reflector failed: red=%g green=%g", redIncrease, greenIncrease)
	}
}
