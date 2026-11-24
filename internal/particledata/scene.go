package particledata

import (
	"encoding/binary"
	"math"
	"time"
)

// Instance is the CPU emission clock and origin for one prepared effect.
// The platform owns every particle slot from creation to death.
type Instance struct {
	Effect       *Effect
	origin       [3]float32
	units        float32
	clock        float64
	fresh, reset bool
	playing      bool
	steps        []Step
	emission     []emission
}

type emission struct {
	carry        float64
	visibleUntil float64
	slots        uint32
}

type Step struct {
	Bytes  [FrameBytes]byte
	Births uint32
	Slots  uint32
	Reset  bool
	Active bool
}

// NewInstance reserves an effect clock. A positive unit scale places the
// effect in world XY. A zero scale uses framebuffer coordinates.
func NewInstance(effect *Effect, origin [3]float32, units float32) *Instance {
	return &Instance{
		Effect: effect, origin: origin, units: units, fresh: true, reset: true, playing: true,
		steps: make([]Step, len(effect.Layers)), emission: make([]emission, len(effect.Layers)),
	}
}

type Space uint8

const (
	ScreenSpace Space = iota
	WorldSpace
	Spaces
)

func (instance *Instance) Space() Space {
	if instance.units > 0 {
		return WorldSpace
	}

	return ScreenSpace
}

func (instance *Instance) SetPosition(origin [3]float32) { instance.origin = origin }

func (instance *Instance) Visible() bool {
	for _, step := range instance.steps {
		if step.Active {
			return true
		}
	}

	return false
}

func (instance *Instance) Restart() {
	instance.clock, instance.fresh, instance.reset, instance.playing = 0, true, true, true
	clear(instance.emission)
}

func (instance *Instance) Stop() {
	instance.playing = false
	clear(instance.steps)
}

func (instance *Instance) Steps() []Step { return instance.steps }

func (instance *Instance) Advance(delta time.Duration, width, height int) {
	if !instance.playing {
		return
	}

	const maxStep = .05

	seconds := min(max(delta.Seconds(), 0), maxStep)
	end := instance.clock + seconds
	duration := instance.Effect.Duration
	wrap := instance.Effect.Loop && end >= duration

	// The same step layout supplies screen viewport or world origin Z and scale.
	sizeX, sizeY := float32(width), float32(height)
	if instance.Space() == WorldSpace {
		sizeX, sizeY = instance.origin[2], instance.units
	}

	for index, layer := range instance.Effect.Layers {
		emitter := &instance.emission[index]

		var reset uint32

		if instance.reset {
			warmSlots := layer.WarmSlots()
			emitter.slots = warmSlots
			emitter.visibleUntil = layer.WarmLife
			reset = 1 + warmSlots
		}

		births := uint32(0)
		if instance.clock < duration && seconds > 0 {
			births = emitter.emit(layer, instance.clock, min(end, duration), instance.fresh)
		}

		if wrap && end > duration {
			births += emitter.emit(layer, 0, end-duration, true)
		}

		births = min(births, layer.Capacity)
		active := emitter.visibleUntil > 0 || births != 0

		emitter.visibleUntil = max(0, emitter.visibleUntil-seconds)
		if births != 0 {
			emitter.visibleUntil = layer.MaxLife + float64(time.Millisecond)/float64(time.Second)
			emitter.slots = min(layer.Capacity, emitter.slots+births)
		}

		step := &instance.steps[index]
		step.Births, step.Reset, step.Active = births, instance.reset, active
		step.Slots = emitter.slots

		put := binary.LittleEndian.PutUint32
		put(step.Bytes[0:4], math.Float32bits(float32(seconds)))
		put(step.Bytes[4:8], math.Float32bits(float32(instance.clock)))
		put(step.Bytes[8:12], math.Float32bits(instance.origin[0]))
		put(step.Bytes[12:16], math.Float32bits(instance.origin[1]))
		put(step.Bytes[16:20], math.Float32bits(sizeX))
		put(step.Bytes[20:24], math.Float32bits(sizeY))
		put(step.Bytes[24:28], births)
		put(step.Bytes[28:32], reset)
	}

	instance.reset = false
	if wrap {
		instance.clock, instance.fresh = end-duration, end == duration
	} else if seconds > 0 {
		instance.clock, instance.fresh = end, false
	}
	// Existing particles only need a short clock tail after one-shot emission.
	if !instance.Effect.Loop {
		instance.clock = min(instance.clock, duration+3601)
	}
}

func (emitter *emission) emit(layer Layer, start, end float64, inclusive bool) uint32 {
	var births uint32

	for _, burst := range layer.Bursts {
		if burst.Time <= end && (burst.Time > start || inclusive && burst.Time == start) {
			births += burst.Count
		}
	}

	activeTime := end - max(start, layer.Delay)
	if activeTime > 0 {
		emitter.carry += activeTime * layer.Rate
		whole := math.Floor(emitter.carry + 1e-9)
		births += uint32(whole)
		emitter.carry = max(0, emitter.carry-whole)
	}

	return births
}

// HasWork reports whether a frame needs particle dispatches even without a
// presentation surface.
func HasWork(instances []*Instance) bool {
	for _, instance := range instances {
		for _, step := range instance.steps {
			if step.Active || step.Reset {
				return true
			}
		}
	}

	return false
}

func DrawSpaces(instances []*Instance) [Spaces]bool {
	var enabled [Spaces]bool
	for _, instance := range instances {
		enabled[instance.Space()] = true
	}

	return enabled
}
