//go:build linux

package lightmapbuild_test

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	lightmapbuild "github.com/karty-game/karty-webgpu/pkg/lightmap/build"
)

func TestHardwareDirectVisibilityAndPadding(t *testing.T) {
	t.Parallel()

	if os.Getenv("KARTY_LIGHTMAP_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAP_GPU_TEST=1 with hardware GPU access")
	}

	encoded, err := lightmapdata.Encode(lightmapfixture.Document())
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	point := lightmap.Point{Position: [3]float32{0, 2, 0}, Color: [3]float32{1, .5, 0}, Intensity: 2, Range: 10}

	var result lightmapbuild.Result
	if browser := os.Getenv("KARTY_LIGHTMAP_BROWSER"); browser != "" {
		result, err = lightmapbuild.BakeBrowser(ctx, asset, point, lightmapbuild.BrowserOptions{Executable: browser})
	} else {
		result, err = lightmapbuild.Bake(ctx, asset, point)
	}

	if err != nil {
		t.Fatal(err)
	}

	t.Logf("hardware adapter=%s cold duration=%s scale=%g", result.Adapter, result.Duration, result.Scale)

	pixels := result.Texture.Levels[0]
	if result.Scale != 2 || pixels[0] != 0 || pixels[1] != 0 || pixels[2] != 0 || pixels[3] != 255 {
		t.Fatalf("blocked receiver/scale: %+v %v", result.Scale, pixels)
	}

	falloff := 1 - 5.0/100

	expected := 255 * (2 / math.Sqrt(5)) * falloff * falloff
	if math.Abs(float64(pixels[4])-expected) > 1 || math.Abs(float64(pixels[5])-expected*.5) > 1 || pixels[6] != 0 || pixels[7] != 255 {
		t.Fatalf("lit receiver got %v want red %g", pixels[4:8], expected)
	}

	for i := range 4 {
		if pixels[8+i] != pixels[4+i] {
			t.Fatal("gutter differs from chart receiver")
		}

		if pixels[12+i] != 0 {
			t.Fatal("empty texel not zero")
		}
	}
}
