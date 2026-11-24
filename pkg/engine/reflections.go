package engine

import (
	"math"

	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
)

type reflectionProbe struct {
	entity                   Entity
	environment              *texturedata.Texture
	origin, minimum, maximum Vector3
	strength                 float32
	volume                   float64
}

func (storage *frameStorage) collectReflectionProbes(world *World) {
	for _, entity := range world.order {
		entry := world.bindings[entity]
		if entry == nil || entry.probe == nil || entry.transform == nil {
			continue
		}

		environment, half, strength := entry.probe.RuntimeReflectionProbe()
		if !texturedata.Valid(environment) || strength <= 0 || !validHalfExtents(half) {
			continue
		}

		origin := entry.transform.Position
		probe := reflectionProbe{
			entity: entity, environment: environment, origin: origin, strength: strength,
			minimum: Vector3{X: origin.X - half.X, Y: origin.Y - half.Y, Z: origin.Z - half.Z},
			maximum: Vector3{X: origin.X + half.X, Y: origin.Y + half.Y, Z: origin.Z + half.Z},
			volume:  float64(half.X) * float64(half.Y) * float64(half.Z),
		}
		storage.probes = append(storage.probes, probe)
	}
}

func validHalfExtents(value Vector3) bool {
	for _, coordinate := range [...]float32{value.X, value.Y, value.Z} {
		if coordinate <= 0 || math.IsNaN(float64(coordinate)) || math.IsInf(float64(coordinate), 0) {
			return false
		}
	}

	return true
}

func (storage *frameStorage) applyReflection(draw *render.DrawCall) {
	if draw.Source || draw.Impostor != nil || draw.ReflectionStrength <= 0 {
		return
	}

	position := Vector3{X: draw.Transform[12], Y: draw.Transform[13], Z: draw.Transform[14]}

	var selected *reflectionProbe

	for index := range storage.probes {
		probe := &storage.probes[index]
		if position.X < probe.minimum.X || position.X > probe.maximum.X ||
			position.Y < probe.minimum.Y || position.Y > probe.maximum.Y ||
			position.Z < probe.minimum.Z || position.Z > probe.maximum.Z {
			continue
		}

		if selected == nil || probe.volume < selected.volume {
			selected = probe
		}
	}

	if selected == nil {
		return
	}

	draw.Reflection = selected.environment
	draw.ReflectionStrength *= selected.strength
	draw.ProbeOrigin = vectorArray(selected.origin)
	draw.ProbeMinimum = vectorArray(selected.minimum)
	draw.ProbeMaximum = vectorArray(selected.maximum)

	draw.ProbeID = selected.entity.index
}
