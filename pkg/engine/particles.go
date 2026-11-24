package engine

import (
	"errors"
	"time"

	"github.com/karty-game/ashura/internal/particledata"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/pkg/particle"
)

const maxParticleSlots = 65536
const maxParticleEffects = 64

var errParticleCapacity = errors.New("engine: particle instance or slot limit exceeded")
var errParticleEffect = errors.New("engine: invalid prepared particle effect or position")

// ParticleInstance controls one effect registered before Run.
// Call its methods on the App loop.
type ParticleInstance struct{ state *particledata.Instance }

// AddParticles places a prepared effect in framebuffer pixels and reserves
// its GPU resources at startup. Register features.Particles before Run.
func (app *App) AddParticles(effect *particle.Effect, originX, originY float32) (*ParticleInstance, error) {
	return app.addParticles(effect, Vector3{X: originX, Y: originY}, 0, particledata.ScreenSpace)
}

// AddWorldParticles places an effect in 3D with camera-facing particles that
// test the scene's depth. The scale is world units per authored pixel.
func (app *App) AddWorldParticles(effect *particle.Effect, origin Vector3, unitsPerPixel float32) (*ParticleInstance, error) {
	return app.addParticles(effect, origin, unitsPerPixel, particledata.WorldSpace)
}

func (app *App) addParticles(effect *particle.Effect, origin Vector3, units float32, space particledata.Space) (*ParticleInstance, error) {
	if app == nil || app.closed {
		return nil, errAppClosed
	}

	if app.driver != nil {
		return nil, errAppRunning
	}

	const largestScale = 1 << 16

	prepared := effect.Prepared()
	if prepared == nil || !validParticleOrigin(origin) || (space == particledata.WorldSpace && !(units > 0 && units <= largestScale)) {
		return nil, errParticleEffect
	}

	var slots uint32
	for _, layer := range prepared.Layers {
		slots += layer.Capacity
	}

	if len(app.particles) >= maxParticleEffects || slots > maxParticleSlots-app.particleSlots {
		return nil, errParticleCapacity
	}

	instance := particledata.NewInstance(prepared, vectorArray(origin), units)
	app.particles = append(app.particles, instance)
	app.particleSlots += slots

	return &ParticleInstance{state: instance}, nil
}

// SetPosition moves the origin and any existing screen-space particles.
func (instance *ParticleInstance) SetPosition(x, y float32) error {
	if instance.state.Space() != particledata.ScreenSpace || !validParticleCoordinate(x) || !validParticleCoordinate(y) {
		return errParticleEffect
	}

	instance.state.SetPosition([3]float32{x, y, 0})

	return nil
}

// SetWorldPosition moves the world origin and any particles relative to it.
func (instance *ParticleInstance) SetWorldPosition(origin Vector3) error {
	if instance.state.Space() != particledata.WorldSpace || !validParticleOrigin(origin) {
		return errParticleEffect
	}

	instance.state.SetPosition(vectorArray(origin))

	return nil
}

// Restart restores the effect's prepared initial particle state and seed.
func (instance *ParticleInstance) Restart() { instance.state.Restart() }

// Stop hides the effect and stops scheduling its simulation until Restart.
func (instance *ParticleInstance) Stop() { instance.state.Stop() }

func validParticleCoordinate(value float32) bool {
	const bound = 1 << 24

	return value >= -bound && value <= bound
}

func validParticleOrigin(origin Vector3) bool {
	return validParticleCoordinate(origin.X) && validParticleCoordinate(origin.Y) && validParticleCoordinate(origin.Z)
}

func (app *App) prepareGPU() error {
	resources := app.computeResources()
	if len(resources.Buffers) > 0 || len(resources.Pipelines) > 0 || len(resources.BindGroups) > 0 {
		if err := app.driver.PrepareCompute(resources); err != nil {
			return err
		}
	}

	return app.prepareParticles()
}

func (app *App) scheduleParticles(frame *render.Frame, delta time.Duration, width, height int) {
	frame.Compute, frame.Particles = app.computeFrame.commands, app.particles
	hasCamera := frame.ViewProjection != ([16]float32{})

	for _, instance := range app.particles {
		instance.Advance(delta, width, height)

		if hasCamera && !frame.WorldParticles && instance.Space() == particledata.WorldSpace {
			frame.WorldParticles = instance.Visible()
		}
	}
}

func (app *App) prepareParticles() error {
	if len(app.particles) == 0 {
		return nil
	}

	host, ok := app.driver.(interface {
		PrepareParticles(instances []*particledata.Instance) error
	})
	if !ok {
		return missingFeature(platform.ParticleFeature)
	}

	return host.PrepareParticles(app.particles)
}
