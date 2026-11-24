package render_test

import (
	"reflect"
	"testing"

	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
)

func TestEffectiveOpaqueMaterialMasksIgnoredLanes(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{Color: [4]float32{.2, .3, .4, 1}, Roughness: .5}
	original := render.EffectiveMaterial(draw)

	var uniforms render.UniformState
	uniforms.Update(draw)
	draw.Color[3], draw.NormalStrength, draw.AOStrength = .2, 99, 88
	draw.HalfLambert, draw.SSBumpStrength = true, 7

	draw.AmbientColors[0][0] = 4
	if render.EffectiveMaterial(draw) != original || uniforms.Update(draw) {
		t.Fatal("opaque alpha, missing-map strength or Source-only fields invalidated Standard")
	}

	draw.Color[0] += .1
	if render.EffectiveMaterial(draw) == original || !uniforms.Update(draw) {
		t.Fatal("meaningful Standard tint mutation was lost")
	}

	draw.Normal = texturedata.Solid([4]byte{128, 128, 255, 255}, texturedata.Linear)
	draw.NormalStrength = .5
	original = render.EffectiveMaterial(draw)
	uniforms.Update(draw)

	draw.NormalStrength = .7
	if render.EffectiveMaterial(draw) == original || !uniforms.Update(draw) {
		t.Fatal("mapped Standard strength mutation was lost")
	}
}

func TestEffectiveSourceMaterialMasksStandardAndAlphaLanes(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{Source: true, Color: [4]float32{.2, .3, .4, 1}, AmbientColors: [3][4]float32{{.2, .3, .4, 1}}}
	original := render.EffectiveMaterial(draw)

	var uniforms render.UniformState
	uniforms.Update(draw)
	draw.Color[3], draw.AmbientColors[0][3] = .2, .7
	draw.Roughness, draw.NormalStrength, draw.AOStrength, draw.SSBumpStrength = .9, 99, 88, 77

	draw.AO = texturedata.Solid([4]byte{0, 0, 0, 255}, texturedata.Linear)
	if render.EffectiveMaterial(draw) != original || uniforms.Update(draw) {
		t.Fatal("Source shader-unused Standard fields/alpha/missing SSBump strength invalidated material")
	}

	draw.AmbientColors[0][1] += .1
	if render.EffectiveMaterial(draw) == original || !uniforms.Update(draw) {
		t.Fatal("meaningful Source ambient RGB mutation was lost")
	}
}

func TestLightmapStateParticipatesInBatchingAndUniformDirtiness(t *testing.T) {
	t.Parallel()

	image := texturedata.Solid([4]byte{128, 128, 128, 255}, texturedata.Linear)
	draw := render.DrawCall{Lightmap: image, LightmapScale: 2}
	key := render.EffectiveMaterial(draw)

	var uniforms render.UniformState
	uniforms.Update(draw)

	draw.LightmapScale = 3
	if render.EffectiveMaterial(draw) == key || !uniforms.Update(draw) {
		t.Fatal("lightmap scale change reused stale material")
	}

	key = render.EffectiveMaterial(draw)

	draw.LightmapDebug = true
	if render.EffectiveMaterial(draw) == key || !uniforms.Update(draw) {
		t.Fatal("UV debug change reused stale material")
	}

	if size := reflect.TypeFor[render.MaterialKey]().Size(); size > 128 {
		t.Fatalf("material keys no longer fit inline: %d", size)
	}
}

func TestRGBEncodingAndUnitsInvalidateOnlyBoundLightmaps(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{}

	var uniforms render.UniformState

	key := render.EffectiveMaterial(draw)
	uniforms.Update(draw)
	draw.LightmapEncoding = texturedata.RNMRGBE

	draw.LightmapDiffuse = true
	if render.EffectiveMaterial(draw) != key || uniforms.Update(draw) {
		t.Fatal("unbound metadata dirtied draw")
	}

	draw.Lightmap = texturedata.Solid([4]byte{128, 64, 32, 128}, texturedata.Linear)
	draw.LightmapScale = 1
	key = render.EffectiveMaterial(draw)
	uniforms.Update(draw)

	draw.LightmapEncoding = texturedata.RNMLinear
	if render.EffectiveMaterial(draw) == key || !uniforms.Update(draw) {
		t.Fatal("encoding mutation reused stale uniforms")
	}

	key = render.EffectiveMaterial(draw)
	uniforms.Update(draw)

	draw.LightmapDiffuse = false
	if render.EffectiveMaterial(draw) == key || !uniforms.Update(draw) {
		t.Fatal("unit mutation reused stale uniforms")
	}
}
