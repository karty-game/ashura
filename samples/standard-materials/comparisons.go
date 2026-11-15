package main

import (
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
)

const modeKey = engine.Key('T')

type materialSurface struct {
	entity engine.Entity
	pair   materialPair
}

type materialDemo struct {
	world        *engine.World
	surfaces     []materialSurface
	failures     sample.Errors
	useSource    bool
	lightingMode int
	mode         *ui.Node
	status       *ui.Node
	normal       *ui.Node
	occlusion    *ui.Node
	ssbump       *ui.Node
	halfLambert  *ui.Node
	lighting     *ui.Node
}

// A mode change changes only component ownership; geometry, texture handles and
// authored values remain shared. Only the selected provider stays attached.
func (demo *materialDemo) setMode(source bool) error {
	if demo.useSource == source {
		return nil
	}

	if err := demo.apply(source); err != nil {
		return err
	}

	demo.useSource = source

	return demo.syncUI()
}

func (demo *materialDemo) apply(source bool) error {
	for _, surface := range demo.surfaces {
		if !demo.world.Alive(surface.entity) {
			return engine.ErrInvalidEntity
		}
	}

	for _, surface := range demo.surfaces {
		if source {
			if err := demo.world.Set(surface.entity, surface.pair.source); err != nil {
				return err
			}

			engine.RemoveComponent[material.Material](demo.world, surface.entity)
		} else {
			if err := demo.world.Set(surface.entity, surface.pair.standard); err != nil {
				return err
			}

			engine.RemoveComponent[material.SourceMaterial](demo.world, surface.entity)
		}
	}

	return nil
}

func (demo *materialDemo) setNormal(enabled bool) error {
	for index := range demo.surfaces {
		demo.surfaces[index].pair.standard.NormalStrength = strength(enabled)
	}

	return demo.updateMaterials()
}

func (demo *materialDemo) setOcclusion(enabled bool) error {
	for index := range demo.surfaces {
		demo.surfaces[index].pair.standard.AOStrength = strength(enabled)
	}

	return demo.updateMaterials()
}

func (demo *materialDemo) setSSBump(enabled bool) error {
	for index := range demo.surfaces {
		demo.surfaces[index].pair.source.SSBumpStrength = strength(enabled)
	}

	return demo.updateMaterials()
}

func (demo *materialDemo) setHalfLambert(enabled bool) error {
	for index := range demo.surfaces {
		demo.surfaces[index].pair.source.HalfLambert = enabled
	}

	return demo.updateMaterials()
}

func (demo *materialDemo) cycleLighting() error {
	const lightingModes = 4

	demo.lightingMode = (demo.lightingMode + 1) % lightingModes

	colors := ambientColors()
	if demo.lightingMode > 0 {
		colors = diagnosticColors()
		for range demo.lightingMode - 1 {
			colors = [3]engine.Color{colors[2], colors[0], colors[1]}
		}
	}

	for index := range demo.surfaces {
		demo.surfaces[index].pair.source.AmbientColors = colors
	}

	return demo.updateMaterials()
}

func (demo *materialDemo) updateMaterials() error {
	if err := demo.apply(demo.useSource); err != nil {
		return err
	}

	return demo.syncUI()
}

func (demo *materialDemo) attachControls(app *engine.App) {
	keys := [...]engine.Key{modeKey, engine.KeyN, engine.KeyO, engine.KeyH, engine.KeyB, engine.KeyR}

	var held [len(keys)]bool

	app.OnUpdateError(func(time.Duration) error {
		if err := demo.failures.Err(); err != nil {
			return err
		}

		for index, key := range keys {
			down := app.Input().KeyDown(key)
			if down && !held[index] {
				if err := demo.keyAction(key); err != nil {
					return err
				}
			}

			held[index] = down
		}

		return nil
	})
}

func (demo *materialDemo) keyAction(key engine.Key) error {
	pair := demo.surfaces[0].pair

	//nolint:exhaustive // Only the material comparison keys are handled.
	switch key {
	case modeKey:
		return demo.setMode(!demo.useSource)
	case engine.KeyN:
		if !demo.useSource {
			return demo.setNormal(pair.standard.NormalStrength == 0)
		}
	case engine.KeyO:
		if !demo.useSource {
			return demo.setOcclusion(pair.standard.AOStrength == 0)
		}
	case engine.KeyH:
		if demo.useSource {
			return demo.setHalfLambert(!pair.source.HalfLambert)
		}
	case engine.KeyB:
		if demo.useSource {
			return demo.setSSBump(pair.source.SSBumpStrength == 0)
		}
	case engine.KeyR:
		if demo.useSource {
			return demo.cycleLighting()
		}
	}

	return nil
}

func strength(enabled bool) float32 {
	if enabled {
		return 1
	}

	return 0
}

func ambientColors() [3]engine.Color {
	const fill = .12

	color := engine.Color{R: fill, G: fill, B: fill, A: 1}

	return [3]engine.Color{color, color, color}
}

//nolint:mnd // The three diagnostic irradiance colors are authored comparison values.
func diagnosticColors() [3]engine.Color {
	return [3]engine.Color{
		{R: .75, G: .20, B: .08, A: 1},
		{R: .08, G: .30, B: .75, A: 1},
		{R: .12, G: .55, B: .20, A: 1},
	}
}
