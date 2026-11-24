package material_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestStandardDefaultsAndIndependentComponents(t *testing.T) {
	t.Parallel()

	shared := material.Standard(material.Options{})
	if shared.Color != engine.ColorWhite() || shared.Roughness != 0 ||
		shared.UVScale != (engine.Vector2{X: 1, Y: 1}) || shared.NormalStrength != 1 || shared.AOStrength != 1 {
		t.Fatalf("unexpected standard defaults: %+v", shared)
	}

	world := engine.NewWorld()
	first := world.Add(shared)
	second := world.Add(shared)
	shared.Color = engine.ColorRed()

	stored, found := engine.Get[material.Material](world, first)
	if !found {
		t.Fatal("material component missing")
	}

	stored.Color = engine.ColorGreen()
	stored.Roughness = .75
	stored.NormalStrength, stored.AOStrength = 0, 0

	color, roughness := stored.RuntimeMaterial()
	albedo, normal, scale, normalStrength := stored.RuntimeTextures()

	_, aoStrength := stored.RuntimeOcclusion()
	if color != engine.ColorGreen() || roughness != .75 || normalStrength != 0 || aoStrength != 0 ||
		albedo != nil || normal != nil || scale != (engine.Vector2{X: 1, Y: 1}) {
		t.Fatal("runtime accessors replaced live component settings with constructor defaults")
	}

	other, found := engine.Get[material.Material](world, second)
	if !found || other.Color != engine.ColorWhite() || other.NormalStrength != 1 || other.AOStrength != 1 {
		t.Fatal("changing one material changed an independently stored component")
	}
}

func preparedTexture(t *testing.T, app *engine.App, space texture.ColorSpace) *texture.Texture {
	t.Helper()

	image, err := app.NewTexture(texture.Data{
		Width: 1, Height: 1, ColorSpace: space, Levels: [][]byte{{128, 128, 255, 255}},
	})
	if err != nil {
		t.Fatal(err)
	}

	return image
}

func TestStandardPreservesPreparedHandlesAndAuthoredValues(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	albedo := preparedTexture(t, app, texture.SRGB)
	normal := preparedTexture(t, app, texture.Linear)
	occlusion := preparedTexture(t, app, texture.Linear)
	options := material.Options{
		Color: engine.Color{R: .2, G: .3, B: .4, A: 1}, Roughness: .6,
		AlbedoTexture: albedo, NormalTexture: normal, AOTexture: occlusion,
		UVScale: engine.Vector2{X: 2, Y: 3}, NormalStrength: .5, AOStrength: .25,
	}
	value := material.Standard(options)
	options.Color, options.UVScale = engine.ColorRed(), engine.Vector2{}

	gotAlbedo, gotNormal, scale, normalStrength := value.RuntimeTextures()

	gotAO, aoStrength := value.RuntimeOcclusion()
	if gotAlbedo != albedo || gotNormal != normal || gotAO != occlusion || scale != (engine.Vector2{X: 2, Y: 3}) ||
		normalStrength != .5 || aoStrength != .25 || value.Roughness != .6 || value.Color.R != .2 {
		t.Fatal("standard material changed authored values or replaced shared prepared handles")
	}
}

func TestSourceDefaultsAndLiveControls(t *testing.T) {
	t.Parallel()

	value := material.Source(material.SourceOptions{})

	neutral := engine.Color{R: .2, G: .2, B: .2, A: 1}
	if value.Color != engine.ColorWhite() || value.UVScale != (engine.Vector2{X: 1, Y: 1}) ||
		value.AmbientColors != [3]engine.Color{neutral, neutral, neutral} || value.HalfLambert || value.SSBumpStrength != 1 {
		t.Fatalf("unexpected Source defaults: %+v", value)
	}

	value.AmbientColors = [3]engine.Color{}
	value.HalfLambert, value.SSBumpStrength = true, 0

	ambient, wrapped, strength := value.RuntimeSource()
	if ambient != ([3]engine.Color{}) || !wrapped || strength != 0 {
		t.Fatal("runtime Source settings replaced direct-only or flat comparison values")
	}
}

func TestSourcePreservesAuthoredAmbientAndPreparedHandles(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	albedo := preparedTexture(t, app, texture.SRGB)
	ssbump := preparedTexture(t, app, texture.Linear)
	colors := [3]engine.Color{engine.ColorRed(), {}, engine.ColorGreen()}
	value := material.Source(material.SourceOptions{
		Color: engine.ColorOrange(), AlbedoTexture: albedo, SSBumpTexture: ssbump,
		AmbientColors: colors, UVScale: engine.Vector2{X: 2, Y: 3},
	})
	colors[0] = engine.ColorWhite()

	gotAlbedo, gotBump, scale, normalStrength := value.RuntimeTextures()
	if gotAlbedo != albedo || gotBump != ssbump || scale != (engine.Vector2{X: 2, Y: 3}) || normalStrength != 0 ||
		value.Color != engine.ColorOrange() || value.AmbientColors[0] != engine.ColorRed() || value.AmbientColors[1] != (engine.Color{}) {
		t.Fatal("Source material replaced authored colors or prepared resource handles")
	}
}

func TestMaterialSidednessDefaultsAndLiveChanges(t *testing.T) {
	t.Parallel()

	standard := material.Standard(material.Options{})

	source := material.Source(material.SourceOptions{})
	if standard.RuntimeCullBackfaces() || source.RuntimeCullBackfaces() {
		t.Fatal("default materials must retain double-sided rendering")
	}

	standard = material.Standard(material.Options{CullBackfaces: true})

	source = material.Source(material.SourceOptions{CullBackfaces: true})
	if !standard.RuntimeCullBackfaces() || !source.RuntimeCullBackfaces() {
		t.Fatal("material constructor lost authored back-face culling")
	}

	standard.CullBackfaces, source.CullBackfaces = false, false
	if standard.RuntimeCullBackfaces() || source.RuntimeCullBackfaces() {
		t.Fatal("live sidedness changes were replaced by constructor settings")
	}
}

func TestStandardAlphaMaskPreservesExplicitCutoffAndTintAlpha(t *testing.T) {
	t.Parallel()

	value := material.DefaultStandard()
	value.AlphaMask, value.AlphaCutoff, value.Color.A = true, 0, 0
	mask, cutoff := value.RuntimeAlphaMask()

	color, _ := value.RuntimeMaterial()
	if !mask || cutoff != 0 || color.A != 0 {
		t.Fatal("explicit zero cutout/tint alpha was defaulted away")
	}

	value = material.Standard(material.Options{AlphaMask: true, AlphaCutoff: .5})
	mask, cutoff = value.RuntimeAlphaMask()

	color, _ = value.RuntimeMaterial()
	if !mask || cutoff != .5 || color.A != 1 {
		t.Fatal("Standard constructor lost cutout options or white default tint")
	}
}
