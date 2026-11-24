package render_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
)

func uniformValue(data [render.MaterialUniformSize]byte, index int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
}

func frameUniformValue(state *render.FrameUniformState, index int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(state.Data[index*4:]))
}

func uniformTexture(t *testing.T) *texturedata.Texture {
	t.Helper()

	image, err := texturedata.Create(texturedata.Data{
		Width: 1, Height: 1, ColorSpace: texturedata.Linear, Levels: [][]byte{{128, 128, 255, 255}},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { texturedata.Release(image) })

	return image
}

func TestStandardUniformsCarryAllEightLightsAndTextureControls(t *testing.T) {
	t.Parallel()

	image := uniformTexture(t)
	draw := render.DrawCall{
		Color: [4]float32{.2, .3, .4, 1}, Roughness: .75, UVScale: [2]float32{2, 3},
		Normal: image, NormalStrength: .5, AO: image, AOStrength: .25,
	}

	frame := render.Frame{CameraPosition: [3]float32{1, 2, 3}}
	for index := range render.MaxLights {
		frame.Lights = append(frame.Lights, render.Light{
			Kind: render.HemisphereLightKind, Direction: [3]float32{0, 1, 0},
			Color: [3]float32{float32(index), .5, .75}, GroundColor: [3]float32{.1, .2, .3}, Intensity: .5,
		})
	}

	data := render.Uniforms(draw)
	for offset, want := range map[int]float32{
		0: .2, 1: .3, 2: .4, 3: 1, 4: .75, 5: .5, 6: 2, 7: 3, 8: .25,
	} {
		if got := uniformValue(data, offset); got != want {
			t.Fatalf("Standard float %d = %v, want %v", offset, got, want)
		}
	}

	var shared render.FrameUniformState
	shared.Update(frame)

	for offset, want := range map[int]float32{16: 1, 17: 2, 18: 3, 19: 8} {
		if got := frameUniformValue(&shared, offset); got != want {
			t.Fatalf("shared float %d = %v, want %v", offset, got, want)
		}
	}

	for index := range render.MaxLights {
		offset := 20 + index*12
		if frameUniformValue(&shared, offset+1) != 1 || frameUniformValue(&shared, offset+3) != 1 ||
			frameUniformValue(&shared, offset+4) != float32(index) || frameUniformValue(&shared, offset+7) != .5 ||
			frameUniformValue(
				&shared,
				offset+8,
			) != .1 || frameUniformValue(&shared, offset+9) != .2 || frameUniformValue(&shared, offset+10) != .3 {
			t.Fatalf("hemisphere record %d was truncated or packed into the wrong shader fields", index)
		}
	}
}

func TestStandardMappingDisableAndMissingMapUniforms(t *testing.T) {
	t.Parallel()

	image := uniformTexture(t)
	draw := render.DrawCall{Normal: image, AO: image, NormalStrength: 1, AOStrength: 1}
	enabled := render.Uniforms(draw)
	draw.NormalStrength, draw.AOStrength = 0, 0

	disabled := render.Uniforms(draw)
	if uniformValue(enabled, 5) != 1 || uniformValue(enabled, 8) != 1 ||
		uniformValue(disabled, 5) != 0 || uniformValue(disabled, 8) != 0 {
		t.Fatal("mapping strengths were replaced with constructor defaults during encoding")
	}

	draw.Normal, draw.AO = nil, nil
	draw.NormalStrength, draw.AOStrength = 1, 1

	missing := render.Uniforms(draw)
	if uniformValue(missing, 5) != 0 || uniformValue(missing, 8) != 0 {
		t.Fatal("missing textures enabled mapping against fallback resources")
	}
}

func TestSourceUniformsIgnoreStandardSettingsAndRetainRNMControls(t *testing.T) {
	t.Parallel()

	image := uniformTexture(t)
	draw := render.DrawCall{
		Source: true, Color: [4]float32{1, .5, .25, 1}, UVScale: [2]float32{2, 3},
		Normal: image, SSBumpStrength: .25, HalfLambert: true,
		AmbientColors: [3][4]float32{{1, 0, 0, 1}, {0, 1, 0, 1}, {0, 0, 1, 1}},
	}
	frame := render.Frame{Lights: []render.Light{{
		Kind: render.DirectionalLightKind, Direction: [3]float32{0, -1, 0}, Color: [3]float32{1, .5, .25}, Intensity: 2,
	}}}

	data := render.Uniforms(draw)
	for offset, want := range map[int]float32{
		1: .25, 2: 1, 4: 1, 5: .5, 6: .25, 7: 1, 8: 2, 9: 3, 12: 1, 17: 1, 22: 1,
	} {
		if got := uniformValue(data, offset); got != want {
			t.Fatalf("Source float %d = %v, want %v", offset, got, want)
		}
	}

	var shared render.FrameUniformState
	shared.Update(frame)

	for offset, want := range map[int]float32{19: 1, 21: -1, 23: 2, 24: 1, 25: .5, 26: .25, 27: 2} {
		if got := frameUniformValue(&shared, offset); got != want {
			t.Fatalf("directional float %d = %v, want %v", offset, got, want)
		}
	}

	draw.AO, draw.AOStrength, draw.NormalStrength, draw.Roughness = image, .5, .75, .9
	if render.Uniforms(draw) != data {
		t.Fatal("Source uniform layout depends on unused Standard shading settings")
	}

	draw.SSBumpStrength, draw.HalfLambert = 0, false

	disabled := render.Uniforms(draw)
	if uniformValue(disabled, 1) != 0 || uniformValue(disabled, 2) != 0 {
		t.Fatal("Source comparison controls did not disable mapped or wrapped lighting")
	}
}

func TestUniformEncodingBoundsLightArrayForBothMaterials(t *testing.T) {
	t.Parallel()

	frame := render.Frame{Lights: make([]render.Light, render.MaxLights+1)}

	var shared render.FrameUniformState
	shared.Update(frame)

	if frameUniformValue(&shared, 19) != render.MaxLights {
		t.Fatal("uniform light count can exceed its bounded WGSL array")
	}
}
