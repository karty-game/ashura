package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/texture"
)

func comparisonFixture(t *testing.T) *materialDemo {
	t.Helper()

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})

	stone, brick, err := loadMaterialsForFormat(app, texture.BC1)
	if err != nil {
		t.Fatal(err)
	}

	stone.setUVScale(engine.Vector2{X: groundTiles, Y: groundTiles})

	world := engine.NewWorld()

	demo := &materialDemo{world: world}
	for _, pair := range []materialPair{stone, brick} {
		entity := world.Add(engine.DefaultTransform(), pair.standard)
		demo.surfaces = append(demo.surfaces, materialSurface{entity: entity, pair: pair})
	}

	return demo
}

func TestMaterialProfileRequiresCompleteTextureSupport(t *testing.T) {
	t.Parallel()

	mobile := []texture.Format{texture.ETC2RGB, texture.ETC2RGBA, texture.ASTC4x4}
	for _, test := range []struct {
		supported []texture.Format
		want      texture.Format
	}{
		{mobile[:2], texture.RGBA8},
		{mobile[2:], texture.RGBA8},
		{mobile, texture.ETC2RGB},
		{[]texture.Format{texture.BC1}, texture.RGBA8},
		{[]texture.Format{texture.BC1, texture.BC7}, texture.BC1},
	} {
		got, err := materialFormat(func(format texture.Format) bool {
			return slices.Contains(test.supported, format)
		})
		if got != test.want || errors.Is(err, errMaterialCompression) != (test.want == texture.RGBA8) {
			t.Fatalf("formats %v selected %v, %v; want %v", test.supported, got, err, test.want)
		}
	}
}

func TestMaterialModeSwitchKeepsOneProviderAndSharedAssets(t *testing.T) {
	t.Parallel()

	demo := comparisonFixture(t)
	for _, source := range []bool{true, false, true, false} {
		if err := demo.setMode(source); err != nil {
			t.Fatal(err)
		}

		for _, surface := range demo.surfaces {
			standard, hasStandard := engine.Get[material.Material](demo.world, surface.entity)

			valve, hasValve := engine.Get[material.SourceMaterial](demo.world, surface.entity)
			if hasStandard == hasValve || hasValve != source {
				t.Fatal("mode switch left a missing or duplicate material provider")
			}

			if surface.pair.standard.AlbedoTexture != surface.pair.source.AlbedoTexture {
				t.Fatal("comparison modes loaded duplicate albedo handles")
			}

			if source {
				if valve.AlbedoTexture != surface.pair.standard.AlbedoTexture || valve.UVScale != surface.pair.standard.UVScale ||
					valve.SSBumpTexture == nil {
					t.Fatal("Valve mode lost its shared albedo, prepared SSBump or authored UV scale")
				}
			} else if *standard != surface.pair.standard {
				t.Fatal("switching back changed the retained Standard settings")
			}
		}
	}
}

func TestComparisonSettingsSurviveModeChanges(t *testing.T) {
	t.Parallel()

	demo := comparisonFixture(t)
	for _, change := range []func() error{
		func() error { return demo.setNormal(false) },
		func() error { return demo.setOcclusion(false) },
		func() error { return demo.setMode(true) },
		func() error { return demo.setSSBump(false) },
		func() error { return demo.setHalfLambert(true) },
		demo.cycleLighting,
		func() error { return demo.setMode(false) },
	} {
		if err := change(); err != nil {
			t.Fatal(err)
		}
	}

	for _, surface := range demo.surfaces {
		standard, ok := engine.Get[material.Material](demo.world, surface.entity)
		if !ok || standard.NormalStrength != 0 || standard.AOStrength != 0 {
			t.Fatal("Source mode replaced the disabled Standard map settings")
		}
	}

	if err := demo.setMode(true); err != nil {
		t.Fatal(err)
	}

	for _, surface := range demo.surfaces {
		valve, ok := engine.Get[material.SourceMaterial](demo.world, surface.entity)
		if !ok || valve.SSBumpStrength != 0 || !valve.HalfLambert || valve.AmbientColors != diagnosticColors() {
			t.Fatal("Standard mode replaced the retained Source comparison settings")
		}
	}
}

func TestSupportedKeyboardModeKeySwitchesBothDirections(t *testing.T) {
	t.Parallel()

	demo := comparisonFixture(t)
	for _, source := range []bool{true, false} {
		if err := demo.keyAction(engine.Key('T')); err != nil {
			t.Fatal(err)
		}

		if demo.useSource != source {
			t.Fatal("the platform-supported T key did not select the other material mode")
		}
	}
}

func TestModeSelectionPreflightsEntitiesBeforeChangingAnyProvider(t *testing.T) {
	t.Parallel()

	demo := comparisonFixture(t)
	demo.world.Remove(demo.surfaces[1].entity)

	if err := demo.setMode(true); !errors.Is(err, engine.ErrInvalidEntity) {
		t.Fatalf("stale presentation must reject a switch: %v", err)
	}

	first := demo.surfaces[0].entity
	if _, ok := engine.Get[material.Material](demo.world, first); !ok || demo.useSource {
		t.Fatal("rejected mode selection changed the surviving Standard surface")
	}

	if _, ok := engine.Get[material.SourceMaterial](demo.world, first); ok {
		t.Fatal("rejected mode selection partially attached Source material")
	}
}
