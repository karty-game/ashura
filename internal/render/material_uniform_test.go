package render_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func uniformValue(data [render.StandardUniformSize]byte, index int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
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

	data := render.Uniforms(draw, frame)
	for offset, want := range map[int]float32{
		16: 1, 17: 2, 18: 3, 19: 8, 20: .2, 21: .3, 22: .4, 23: 1,
		24: .75, 25: .5, 26: 2, 27: 3, 28: .25,
	} {
		if got := uniformValue(data, offset); got != want {
			t.Fatalf("Standard float %d = %v, want %v", offset, got, want)
		}
	}

	for index := range render.MaxLights {
		offset := 44 + index*12
		if uniformValue(data, offset+1) != 1 || uniformValue(data, offset+3) != 1 ||
			uniformValue(data, offset+4) != float32(index) || uniformValue(data, offset+7) != .5 ||
			uniformValue(data, offset+8) != .1 || uniformValue(data, offset+9) != .2 || uniformValue(data, offset+10) != .3 {
			t.Fatalf("hemisphere record %d was truncated or packed into the wrong shader fields", index)
		}
	}
}

func TestStandardMappingDisableAndMissingMapUniforms(t *testing.T) {
	t.Parallel()

	image := uniformTexture(t)
	draw := render.DrawCall{Normal: image, AO: image, NormalStrength: 1, AOStrength: 1}
	frame := render.Frame{}
	enabled := render.Uniforms(draw, frame)
	draw.NormalStrength, draw.AOStrength = 0, 0

	disabled := render.Uniforms(draw, frame)
	if uniformValue(enabled, 25) != 1 || uniformValue(enabled, 28) != 1 ||
		uniformValue(disabled, 25) != 0 || uniformValue(disabled, 28) != 0 {
		t.Fatal("mapping strengths were replaced with constructor defaults during encoding")
	}

	draw.Normal, draw.AO = nil, nil
	draw.NormalStrength, draw.AOStrength = 1, 1

	missing := render.Uniforms(draw, frame)
	if uniformValue(missing, 25) != 0 || uniformValue(missing, 28) != 0 {
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

	data := render.Uniforms(draw, frame)
	for offset, want := range map[int]float32{
		16: 1, 17: .25, 18: 1, 20: 1, 21: .5, 22: .25, 23: 1, 24: 2, 25: 3,
		28: 1, 33: 1, 38: 1, 41: -1, 43: 2, 44: 1, 45: .5, 46: .25, 47: 2,
	} {
		if got := uniformValue(data, offset); got != want {
			t.Fatalf("Source float %d = %v, want %v", offset, got, want)
		}
	}

	draw.AO, draw.AOStrength, draw.NormalStrength, draw.Roughness = image, .5, .75, .9
	frame.CameraPosition = [3]float32{5, 6, 7}

	frame.Lights[0].GroundColor = [3]float32{1, 1, 1}
	if render.Uniforms(draw, frame) != data {
		t.Fatal("Source uniform layout depends on unused Standard shading settings")
	}

	draw.SSBumpStrength, draw.HalfLambert = 0, false

	disabled := render.Uniforms(draw, frame)
	if uniformValue(disabled, 17) != 0 || uniformValue(disabled, 18) != 0 {
		t.Fatal("Source comparison controls did not disable mapped or wrapped lighting")
	}
}

func TestUniformEncodingBoundsLightArrayForBothMaterials(t *testing.T) {
	t.Parallel()

	frame := render.Frame{Lights: make([]render.Light, render.MaxLights+1)}
	for _, source := range []bool{false, true} {
		data := render.Uniforms(render.DrawCall{Source: source}, frame)

		countOffset := 19
		if source {
			countOffset = 16
		}

		if uniformValue(data, countOffset) != render.MaxLights {
			t.Fatal("uniform light count can exceed its bounded WGSL array")
		}
	}
}
