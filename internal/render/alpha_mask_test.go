package render_test

import (
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/render"
)

func TestAlphaMaskUniformAndRetainedMaterialChanges(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{Color: [4]float32{.2, .3, .4, .75}, AlphaMask: true, AlphaCutoff: .5}
	frame := render.Frame{}
	key := render.EffectiveMaterial(draw)

	var state render.UniformState
	if !state.Update(draw, frame) || uniformValue(state.Data, 39) != .5 || uniformValue(state.Data, 43) != 1 ||
		uniformValue(state.Data, 23) != .75 {
		t.Fatal("mask cutoff or tint alpha omitted from Standard uniforms")
	}

	if state.Update(draw, frame) {
		t.Fatal("unchanged cutout material triggered an upload")
	}

	draw.Color[3] = .25
	if render.EffectiveMaterial(draw) == key || !state.Update(draw, frame) {
		t.Fatal("masked tint alpha mutation reused stale state")
	}

	key = render.EffectiveMaterial(draw)

	draw.AlphaCutoff = 0
	if render.EffectiveMaterial(draw) == key || !state.Update(draw, frame) || uniformValue(state.Data, 39) != 0 ||
		uniformValue(state.Data, 43) != 1 {
		t.Fatal("explicit zero cutoff disabled the mask or reused stale state")
	}

	key = render.EffectiveMaterial(draw)

	draw.AlphaMask = false
	if render.EffectiveMaterial(draw) == key || !state.Update(draw, frame) || uniformValue(state.Data, 39) != 0 ||
		uniformValue(state.Data, 43) != 0 {
		t.Fatal("disabling cutout retained mask state")
	}

	key = render.EffectiveMaterial(draw)

	draw.AlphaCutoff, draw.Color[3] = .9, .1
	if render.EffectiveMaterial(draw) != key || state.Update(draw, frame) {
		t.Fatal("opaque alpha/cutoff changed batches or uploaded unused uniforms")
	}
}

func TestAlphaMaskAdmissionAndUnsupportedShaders(t *testing.T) {
	t.Parallel()

	for _, value := range []float32{0, .5, 1} {
		draw := render.DrawCall{AlphaMask: true, AlphaCutoff: value, Color: [4]float32{1, 1, 1, value}}
		if err := render.ValidateAlphaMask(draw); err != nil {
			t.Fatal("valid endpoint rejected", value, err)
		}
	}

	for _, value := range []float32{-.1, 1.1, float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		for _, cutoff := range []bool{false, true} {
			draw := render.DrawCall{AlphaMask: true, AlphaCutoff: .5, Color: [4]float32{1, 1, 1, 1}}
			if cutoff {
				draw.AlphaCutoff = value
			} else {
				draw.Color[3] = value
			}

			if err := render.ValidateAlphaMask(draw); err == nil {
				t.Fatal("nonfinite/out-of-range cutout value admitted", value, cutoff)
			}

			draw.AlphaMask = false
			if err := render.ValidateAlphaMask(draw); err != nil {
				t.Fatal("opaque unused value rejected", value, cutoff, err)
			}
		}
	}

	for _, draw := range []render.DrawCall{{Source: true}, {Impostor: &impostordata.Document{}}} {
		draw.AlphaMask, draw.AlphaCutoff, draw.Color[3] = true, .5, 1
		if err := render.ValidateAlphaMask(draw); err == nil {
			t.Fatal("unsupported shader silently admitted cutout")
		}
	}
}

func TestAlphaMaskIsExcludedFromPointShadowCasters(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{ID: 1, Vertices: []render.Vertex{{}, {}, {}}, Indices: []uint32{0, 1, 2}}
	frame := render.Frame{Draws: []render.DrawCall{draw}}

	var state render.PointShadowState

	light := render.Light{GroundColor: [3]float32{10}, ShadowResolution: 256}
	if !render.CastsPointShadow(draw) || !state.Update(frame, light) || state.Update(frame, light) {
		t.Fatal("opaque caster did not retain unchanged shadow state")
	}

	frame.Draws[0].AlphaMask = true
	if render.CastsPointShadow(frame.Draws[0]) || !state.Update(frame, light) {
		t.Fatal("mask transition retained a solid-card shadow caster")
	}

	frame.Draws[0].Transform[12] = 2
	if state.Update(frame, light) {
		t.Fatal("moving excluded cutout dirtied shadow depth")
	}

	frame.Draws[0].AlphaMask = false
	if !state.Update(frame, light) {
		t.Fatal("restored opaque caster did not invalidate depth")
	}
}

func TestAlphaMaskPreservesExactCutoffBits(t *testing.T) {
	t.Parallel()

	cutoff := float32(1) / 255
	draw := render.DrawCall{AlphaMask: true, AlphaCutoff: cutoff, Color: [4]float32{1, 1, 1, 1}}

	var state render.UniformState
	state.Update(draw, render.Frame{})

	if math.Float32bits(uniformValue(state.Data, 39)) != math.Float32bits(cutoff) || uniformValue(state.Data, 43) != 1 {
		t.Fatal("mask encoding rounded cutoff or lost enable flag")
	}

	key := render.EffectiveMaterial(draw)

	draw.AlphaCutoff = math.Nextafter32(cutoff, 1)
	if render.EffectiveMaterial(draw) == key || !state.Update(draw, render.Frame{}) ||
		math.Float32bits(uniformValue(state.Data, 39)) != math.Float32bits(draw.AlphaCutoff) {
		t.Fatal("adjacent float32 cutoff reused stale batching/uniform state")
	}
}
