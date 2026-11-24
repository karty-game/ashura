//go:build !js

//nolint:mnd // Noise coefficients match the WGSL hash and curl implementation.
package build

import (
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/internal/particledata"
)

func (warm *warmLayer) drift(particle *warmParticle, frameClock, strength float32) {
	octaves := int(warmFloat(warm.config, 19))
	if octaves == 0 || strength == 0 {
		return
	}

	frequency := warmFloat(warm.config, 17)
	scroll := frameClock * warmFloat(warm.config, 18) * frequency
	mode := warmFloat(warm.config[particledata.SpatialOffset:], 0)
	volume := mode == particledata.SpatialSphere || mode == particledata.SpatialCone
	phase := scroll + particle.Phase*37

	if volume {
		phase += particle.Position[2] * frequency
	}

	const hashStep = uint32(0x9e3779b9)

	seed := binary.LittleEndian.Uint32(warm.config[24*4:]) + hashStep
	x, y := particle.Position[0]*frequency+phase, particle.Position[1]*frequency+scroll*.773
	deltaX, deltaY := warmCurl(x, y, seed, octaves)
	strength *= warmStep
	particle.Position[0] += deltaX * strength
	particle.Position[1] += deltaY * strength

	if volume {
		particle.Position[2] += deltaX * (particle.Phase*2 - 1) * strength
	}
}

func warmCurl(pointX, pointY float32, seed uint32, octaves int) (float32, float32) {
	const span = .35

	var deltaX, deltaY float32

	amplitude := float32(1)
	for range octaves {
		deltaX += amplitude / (2 * span) * (warmNoise(pointX, pointY+span, seed) - warmNoise(pointX, pointY-span, seed))
		deltaY += amplitude / (2 * span) * (warmNoise(pointX-span, pointY, seed) - warmNoise(pointX+span, pointY, seed))
		pointX *= 2
		pointY *= 2
		amplitude *= .5
	}

	return deltaX, deltaY
}

func warmNoise(pointX, pointY float32, seed uint32) float32 {
	cellX, cellY := float32(math.Floor(float64(pointX))), float32(math.Floor(float64(pointY)))
	fade := func(f float32) float32 { return f * f * f * (f*(f*6-15) + 10) }
	hash := func(px, py float32) float32 {
		// Match WGSL's wrapping bit casts of the signed cell coordinates.
		x := uint32(int32(px)) * 0x9e3779b9 //nolint:gosec // Intentional bit reinterpretation.
		y := uint32(int32(py)) * 0x85ebca6b //nolint:gosec // Intentional bit reinterpretation.

		return warmRandom((x ^ y ^ seed) + particleRandomStep)
	}
	weightX, weightY := fade(pointX-cellX), fade(pointY-cellY)
	top := warmMix(hash(cellX, cellY), hash(cellX+1, cellY), weightX)
	bottom := warmMix(hash(cellX, cellY+1), hash(cellX+1, cellY+1), weightX)

	return warmMix(top, bottom, weightY)
}
