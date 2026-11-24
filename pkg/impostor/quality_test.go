//go:build !js

package impostor_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/impostor"
)

func TestQualityEditsPreservePreparedAssetAndRejectInvalidWork(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	asset, err := impostor.Prepared("fixture", fixtureAsset(t)).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	world := engine.NewWorld()

	instance, err := asset.Instantiate(world, engine.DefaultTransform())
	if err != nil {
		t.Fatal(err)
	}

	stored, _ := engine.Get[impostor.Impostor](world, instance.Entity())

	original := append([]byte(nil), asset.RuntimeImpostor().Uniform...)
	color, normal, _, _ := asset.RuntimeTextures()

	for _, quality := range []impostor.Quality{
		impostor.DefaultQuality(),
		{MaxCorrections: 0}, {MaxCorrections: 1}, {MaxCorrections: 2, MipBias: 1},
		{Adaptive: true, FarPixels: 8, NearPixels: 64, MaxCorrections: 1, MipBias: -.5},
	} {
		if err := quality.Validate(); err != nil {
			t.Fatal(err)
		}

		if err := asset.SetQuality(quality); err != nil || asset.Quality() != quality || stored.Quality() != quality {
			t.Fatalf("quality edit failed: %v", err)
		}

		if !bytes.Equal(original, asset.RuntimeImpostor().Uniform) {
			t.Fatal("quality edit repacked the prepared descriptor")
		}

		currentColor, currentNormal, _, _ := asset.RuntimeTextures()
		if color != currentColor || normal != currentNormal {
			t.Fatal("quality edit replaced resident textures")
		}
	}

	previous := asset.Quality()

	for _, quality := range []impostor.Quality{
		{MaxCorrections: -1}, {MaxCorrections: 3},
		{MipBias: 5}, {MipBias: float32(math.NaN())}, {MipBias: float32(math.Inf(1))},
		{Adaptive: true, FarPixels: 0, NearPixels: 128},
		{Adaptive: true, FarPixels: 16, NearPixels: 31},
		{Adaptive: true, FarPixels: 16, NearPixels: 32769},
		{Adaptive: true, FarPixels: float32(math.NaN()), NearPixels: 128},
		{Adaptive: true, FarPixels: 16, NearPixels: float32(math.Inf(1))},
	} {
		if err := quality.Validate(); err == nil {
			t.Fatal("invalid quality passed pure validation")
		}

		if err := asset.SetQuality(quality); err == nil || asset.Quality() != previous {
			t.Fatalf("invalid quality was admitted or partially applied: %+v", quality)
		}
	}

	instance.Remove()

	_ = asset.Unload()
	if err := asset.SetQuality(impostor.DefaultQuality()); err == nil {
		t.Fatal("edited an unloaded asset")
	}
}
