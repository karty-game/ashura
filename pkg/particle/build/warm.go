//go:build !js

//nolint:mnd // Numeric constants mirror the packed configuration and particle shader.
package build

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/particledata"
)

const warmStep = 1.0 / 60
const particleRandomStep = 0x6d2b79f5

// warmParticle follows the storage-buffer layout in particles.wgsl.
type warmParticle struct {
	Position        [3]float32
	Phase           float32
	Velocity        [3]float32
	Spin            float32
	Age, Life, Size float32
	Rotation        float32
}

type warmLayer struct {
	config []byte
	rng    uint32
}

func bakeWarm(layer *particledata.Layer, duration float64) error {
	const maxDuration = 10
	if duration > maxDuration || layer.Capacity > particledata.MaxWarmParticles {
		return fmt.Errorf("prewarm exceeds %d seconds or %d slots: %w", maxDuration, particledata.MaxWarmParticles, errUnsupported)
	}

	if layer.Rate == 0 || layer.Delay >= duration {
		return nil
	}

	sim := warmLayer{config: layer.Config, rng: binary.LittleEndian.Uint32(layer.Config[24*4:])}
	slots := make([]warmParticle, layer.Capacity)

	var carry float64

	for frame := range max(1, int(math.Round(duration/warmStep))) {
		start := float64(frame) * warmStep
		for index := range slots {
			sim.advance(&slots[index], float32(start))
		}

		elapsed := max(0, min(start+warmStep, duration)-max(start, layer.Delay))
		carry += elapsed * layer.Rate

		births := int(math.Floor(carry + 1e-9))
		carry = max(0, carry-float64(births))
		sim.fill(slots, births, float32(start))
	}

	return writeWarm(layer, slots, sim.rng)
}

func writeWarm(layer *particledata.Layer, slots []warmParticle, seed uint32) error {
	live := slots[:0]

	for _, slot := range slots {
		if slot.Life > 0 {
			live = append(live, slot)
			layer.WarmLife = max(layer.WarmLife, float64(slot.Life)-float64(slot.Age))
		}
	}

	if len(live) == 0 {
		return nil
	}

	layer.Warm = make([]byte, particledata.StateHeader+len(live)*particledata.ParticleBytes)
	binary.LittleEndian.PutUint32(layer.Warm, seed)
	_, err := binary.Encode(layer.Warm[particledata.StateHeader:], binary.LittleEndian, live)

	return err
}

func (warm *warmLayer) fill(slots []warmParticle, births int, clock float32) {
	for index := range slots {
		if births == 0 {
			return
		}

		if slots[index].Life == 0 {
			slots[index] = warm.spawn(clock)
			births--
		}
	}
}

func warmFloat(data []byte, word int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[word*4:]))
}

func (warm *warmLayer) next() float32 {
	warm.rng += particleRandomStep

	return warmRandom(warm.rng)
}

func warmRandom(word uint32) float32 {
	value := (word ^ (word >> 15)) * (1 | word)
	value = (value + (value^(value>>7))*(61|value)) ^ value

	return float32((value^(value>>14))>>8) / 16777216
}

func (warm *warmLayer) spawn(clock float32) warmParticle {
	first, second, third := warm.next(), warm.next(), warm.next()
	random := func(offset int) float32 {
		return warmMix(warmFloat(warm.config, offset), warmFloat(warm.config, offset+1), warm.next())
	}
	life, speed, size, rotation, spin := random(0), random(2), random(4), random(6), random(8)

	// Match the five reserved draws in the runtime spawn shader.
	const reserved = uint32(particleRandomStep * 5 % (1 << 32))

	warm.rng += reserved

	var phase float32
	if warmFloat(warm.config, 19) != 0 {
		phase = warm.next()
	}

	position, direction := warm.shape(first, second, third)
	placement := warm.config[particledata.PlacementOffset:]

	if rise := warmFloat(placement, 2); rise > 0 {
		position[1] -= max(0, clock*(rise-.5*warmFloat(placement, 3)*clock))
	}

	for axis := range direction {
		direction[axis] *= speed
	}

	return warmParticle{Position: position, Velocity: direction, Phase: phase, Spin: spin,
		Life: life, Size: size, Rotation: rotation}
}

func (warm *warmLayer) shape(first, second, third float32) ([3]float32, [3]float32) {
	spatial := warm.config[particledata.SpatialOffset:]
	angle := first * (2 * math.Pi)
	azimuth := warmHeading(angle)
	radius := warmFloat(spatial, 1)

	switch warmFloat(spatial, 0) {
	case particledata.SpatialSphere:
		height := second*2 - 1
		ring := warmSqrt(1 - height*height)
		direction := [3]float32{ring * azimuth[0], height, ring * azimuth[1]}
		radius *= float32(math.Cbrt(float64(third)))

		return [3]float32{direction[0] * radius, direction[1] * radius, direction[2] * radius}, direction
	case particledata.SpatialCone:
		axis := warmHeading(warmFloat(warm.config, 14))
		cosine := warmMix(warmFloat(spatial, 2), 1, second)
		ring := warmSqrt(1 - cosine*cosine)
		disk := warmHeading(third * (2 * math.Pi))
		distance := radius * warmSqrt(warm.next())
		position := [3]float32{-axis[1] * disk[0] * distance, axis[0] * disk[0] * distance, disk[1] * distance}
		direction := [3]float32{
			axis[0]*cosine - axis[1]*azimuth[0]*ring,
			axis[1]*cosine + axis[0]*azimuth[0]*ring, azimuth[1] * ring,
		}

		return position, direction
	default:
		return warm.planar(first, second, third)
	}
}

func (warm *warmLayer) planar(first, second, third float32) ([3]float32, [3]float32) {
	const upAngle = -math.Pi / 2 // Source coordinates use positive Y down.

	kind, width := warmFloat(warm.config, 12), warmFloat(warm.config, 13)
	inner, span := warmFloat(warm.config, 14), warmFloat(warm.config, 15)
	surface := warmFloat(warm.config, 25) != 0

	switch kind {
	case 1, 2:
		angle := first * span
		if kind == 2 {
			angle += inner - span*.5
			inner = 0
		}

		radius := width
		if !surface {
			radius = warmSqrt(warmMix(inner*inner, width*width, second))
		}

		direction := warmHeading(angle)
		position := [3]float32{radius * direction[0], radius * direction[1], 0}

		if radius == 0 && kind == 1 {
			direction = warmHeading(third * (2 * math.Pi))
		}

		return position, direction
	case 3:
		position := [3]float32{(first - .5) * width, (second - .5) * inner, 0}
		if surface {
			position = warmRectEdge(first, width, inner)
		}

		return position, warmHeading(upAngle)
	case 4:
		return [3]float32{(first - .5) * width, 0, 0}, warmHeading(upAngle)
	default:
		return [3]float32{}, warmHeading(third * (2 * math.Pi))
	}
}

func warmRectEdge(first, width, height float32) [3]float32 {
	halfWidth, halfHeight := width*.5, height*.5

	distance := first * 2 * (width + height)
	switch {
	case distance < width:
		return [3]float32{distance - halfWidth, -halfHeight, 0}
	case distance < width+height:
		return [3]float32{halfWidth, distance - width - halfHeight, 0}
	case distance < 2*width+height:
		return [3]float32{halfWidth - (distance - width - height), halfHeight, 0}
	default:
		return [3]float32{-halfWidth, halfHeight - (distance - 2*width - height), 0}
	}
}

func warmHeading(angle float32) [3]float32 {
	sine, cosine := math.Sincos(float64(angle))

	return [3]float32{float32(cosine), float32(sine), 0}
}

func warmSqrt(value float32) float32 { return float32(math.Sqrt(float64(max(0, value)))) }

func warmMix(a, b, t float32) float32 { return a + (b-a)*t }

func (warm *warmLayer) advance(particle *warmParticle, clock float32) {
	if particle.Life == 0 {
		return
	}

	age := particle.Age + warmStep
	if age >= particle.Life {
		particle.Life = 0

		return
	}

	const valuesPerSample = 8

	sample := min(particle.Age/particle.Life, 1) * (particledata.Samples - 1)
	low := int(sample)
	high := min(low+1, particledata.Samples-1)
	track := func(index int) float32 {
		base := particledata.ConfigVectors*4 + index

		before := warmFloat(warm.config, base+low*valuesPerSample)
		after := warmFloat(warm.config, base+high*valuesPerSample)

		return warmMix(before, after, sample-float32(low))
	}
	decay := max(0, 1-track(2)*warmStep)
	gravity := [3]float32{warmFloat(warm.config, 10), warmFloat(warm.config, 11), 0}

	for axis := range gravity {
		particle.Velocity[axis] = (particle.Velocity[axis] + gravity[axis]*warmStep) * decay
		particle.Position[axis] += particle.Velocity[axis] * warmStep
	}

	warm.drift(particle, clock, track(3))
	particle.Rotation += (particle.Spin + track(1)) * warmStep
	particle.Age = age
}
