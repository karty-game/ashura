package render_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/render"
)

func TestImpostorQualityParticipatesInUniformsAndBatching(t *testing.T) {
	t.Parallel()

	draw := render.DrawCall{Impostor: &impostordata.Document{Mode: impostordata.ModeOctahedral}, ImpostorQuality: [4]float32{16, 128, 2, 0}}

	var state render.UniformState
	if !state.Update(draw, render.Frame{}) || state.Update(draw, render.Frame{}) {
		t.Fatal("unchanged quality invalidated uniforms")
	}

	for index := range draw.ImpostorQuality {
		key := render.EffectiveMaterial(draw)

		draw.ImpostorQuality[index] += .5
		if render.EffectiveMaterial(draw) == key || !state.Update(draw, render.Frame{}) {
			t.Fatal("quality mutation reused stale batch or uniform")
		}

		value := math.Float32frombits(binary.LittleEndian.Uint32(state.Data[(24+index)*4:]))
		if value != draw.ImpostorQuality[index] || state.Update(draw, render.Frame{}) {
			t.Fatal("quality lane not uploaded exactly once")
		}
	}

	draw.Impostor = nil
	key := render.EffectiveMaterial(draw)
	state.Update(draw, render.Frame{})

	draw.ImpostorQuality = [4]float32{4, 32, -3, 1}
	if render.EffectiveMaterial(draw) != key || state.Update(draw, render.Frame{}) {
		t.Fatal("impostor-only quality invalidated an ordinary material")
	}
}
