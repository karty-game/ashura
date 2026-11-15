package render_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestSourceRNMSnapshotsTexturesScaleAndReplacesFallback(t *testing.T) {
	t.Parallel()
	first, second := uniformTexture(t), uniformTexture(t)
	draw := render.DrawCall{Source: true, RNM: [3]*texturedata.Texture{first, first, first}, RNMScale: 2}

	original := render.EffectiveMaterial(draw)

	var state render.UniformState
	state.Update(draw, render.Frame{})

	if uniformValue(state.Data, 19) != 2 {
		t.Fatal("Source RNM scale missing from uniform")
	}

	draw.AmbientColors[0][0] = 88

	if render.EffectiveMaterial(draw) != original || state.Update(draw, render.Frame{}) {
		t.Fatal("fallback ambient affects mapped Source lighting")
	}

	draw.RNMScale = 3

	if render.EffectiveMaterial(draw) == original || !state.Update(draw, render.Frame{}) {
		t.Fatal("RNM scale mutation reused stale state")
	}

	original = render.EffectiveMaterial(draw)

	draw.RNMEncoding = texturedata.RNMRGBE
	if render.EffectiveMaterial(draw) == original || !state.Update(draw, render.Frame{}) || uniformValue(state.Data, 26) != 1 {
		t.Fatal("RGBE encoding mutation reused stale state")
	}

	original = render.EffectiveMaterial(draw)
	draw.RNM[2] = second

	if render.EffectiveMaterial(draw) == original {
		t.Fatal("RNM third texture identity missing from batching")
	}

	// Once detached, fallback coefficients participate again.
	draw.RNM = [3]*texturedata.Texture{}

	state.Update(draw, render.Frame{})

	if uniformValue(state.Data, 19) != 0 {
		t.Fatal("detached RNM still samples irradiance maps")
	}

	draw.AmbientColors[0][0]++

	if !state.Update(draw, render.Frame{}) {
		t.Fatal("fallback ambient mutation ignored after detach")
	}
}

//nolint:paralleltest // Go 1.27 forbids AllocsPerRun inside parallel tests.
func TestSourceRNMUnchangedBatchKeyDoesNotAllocate(t *testing.T) {
	image := uniformTexture(t)
	draw := render.DrawCall{Source: true, RNM: [3]*texturedata.Texture{image, image, image}, RNMScale: 2}

	keys := make(map[render.MaterialKey]bool)
	keys[render.EffectiveMaterial(draw)] = true

	if allocations := testing.AllocsPerRun(100, func() {
		if !keys[render.EffectiveMaterial(draw)] {
			t.Fatal("changed key")
		}
	}); allocations != 0 {
		t.Fatalf("Source key allocated %.1f times per lookup", allocations)
	}
}
