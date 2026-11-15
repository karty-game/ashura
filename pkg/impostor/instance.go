package impostor

import (
	"math"

	"github.com/karty-game/karty-webgpu/pkg/engine"
)

// Instance is one entity sharing its source impostor's prepared resources.
type Instance struct {
	asset  *Impostor
	world  *engine.World
	entity engine.Entity
}

// Instantiate creates an independently transformed instance. Camera-facing
// selection and quad orientation run in the approved GPU path, not on the CPU.
func (value *Impostor) Instantiate(world *engine.World, transform engine.Transform) (*Instance, error) {
	if value == nil || !value.loaded || value.app.Closed() {
		return nil, errUnavailable
	}

	if world == nil || !validTransform(transform) {
		return nil, errWorld
	}

	value.pruneInstances()
	instance := &Instance{asset: value, world: world}
	instance.entity = world.Add(value, transform)
	value.instances[instance] = struct{}{}

	return instance, nil
}

// Entity returns the instance's entity handle for ordinary component updates.
func (instance *Instance) Entity() engine.Entity { return instance.entity }

// Remove detaches this instance without unloading shared atlas resources.
func (instance *Instance) Remove() bool {
	if instance == nil || instance.world == nil {
		return false
	}

	removed := instance.world.Remove(instance.entity)
	delete(instance.asset.instances, instance)
	instance.world = nil

	return removed
}

func (value *Impostor) pruneInstances() {
	for instance := range value.instances {
		if !instance.world.Alive(instance.entity) {
			delete(value.instances, instance)
			instance.world = nil
		}
	}
}

func validTransform(transform engine.Transform) bool {
	for _, value := range [9]float32{transform.Position.X, transform.Position.Y, transform.Position.Z,
		transform.Rotation.X, transform.Rotation.Y, transform.Rotation.Z, transform.Scale.X, transform.Scale.Y, transform.Scale.Z} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}

	return transform.Scale == (engine.Vector3{}) || float64(transform.Scale.X)*float64(transform.Scale.Y)*float64(transform.Scale.Z) > 0
}
