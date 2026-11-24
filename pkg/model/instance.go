package model

import (
	"errors"
	"math"
	"reflect"

	"github.com/karty-game/ashura/pkg/engine"
)

var errPartRange = errors.New("model instance range is outside its prepared parts")

// Instance is one logical model instance containing all flattened primitives.
// Each entity has an independent Transform and a by-value material.
type Instance struct {
	model    *Model
	world    *engine.World
	entities []engine.Entity
}

// Instantiate adds all prepared primitives to world with the supplied root
// transform. Node transforms were already baked offline. Source primitives and
// textures are shared; removing one instance leaves other instances intact.
func (model *Model) Instantiate(world *engine.World, transform engine.Transform) (*Instance, error) {
	if model == nil {
		return nil, errUnavailable
	}

	return model.instantiate(world, transform, 0, len(model.parts), true, nil)
}

// InstantiateWithMaterial assigns a copy of surface to each part instead of
// its authored material. The override supplies the full shading and coverage.
func (model *Model) InstantiateWithMaterial(
	world *engine.World, transform engine.Transform, surface engine.MaterialProvider,
) (*Instance, error) {
	if model == nil {
		return nil, errUnavailable
	}

	value := reflect.ValueOf(surface)
	if !value.IsValid() || value.Kind() == reflect.Pointer && value.IsNil() {
		return nil, engine.ErrInvalidComponent
	}

	return model.instantiate(world, transform, 0, len(model.parts), true, surface)
}

// InstantiateRange adds an existing contiguous range of prepared parts without
// converting, slicing or rebuilding their geometry. Remove the returned instance
// explicitly when no longer wanted. Historical instance pruning stays in Unload
// and full Instantiate so this bounded operation never scans previous ranges.
func (model *Model) InstantiateRange(world *engine.World, transform engine.Transform, first, count int) (*Instance, error) {
	return model.instantiate(world, transform, first, count, false, nil)
}

func (model *Model) instantiate(
	world *engine.World, transform engine.Transform, first, count int, prune bool, override engine.MaterialProvider,
) (*Instance, error) {
	if model == nil || !model.loaded || model.app.Closed() {
		return nil, errUnavailable
	}

	if world == nil {
		return nil, errWorld
	}

	if !validRootTransform(transform) {
		return nil, errRootTransform
	}

	if first < 0 || count <= 0 || first > len(model.parts) || count > len(model.parts)-first {
		return nil, errPartRange
	}

	if prune {
		model.pruneInstances()
	}

	instance := &Instance{model: model, world: world, entities: make([]engine.Entity, 0, count)}

	committed := false
	defer func() {
		if !committed {
			for _, entity := range instance.entities {
				world.Remove(entity)
			}
		}
	}()

	for _, source := range model.parts[first : first+count] {
		surface := override
		if surface == nil {
			surface = source.material
		}

		instance.entities = append(instance.entities, world.Add(source, transform, surface))
	}

	model.instances[instance] = struct{}{}
	committed = true

	return instance, nil
}

// SetTransform updates all attached part transforms in place. The transform
// follows the same rules as Instantiate. Removed parts and detached Transform
// components are skipped.
func (instance *Instance) SetTransform(transform engine.Transform) error {
	if instance == nil || instance.world == nil || !instance.model.loaded || instance.model.app.Closed() {
		return errUnavailable
	}

	if !validRootTransform(transform) {
		return errRootTransform
	}

	for _, entity := range instance.entities {
		if part := instance.world.Transform(entity); part != nil {
			*part = transform
		}
	}

	return nil
}

// Entities returns an independent list of the instance's entity handles.
func (instance *Instance) Entities() []engine.Entity {
	if instance == nil {
		return nil
	}

	return append([]engine.Entity(nil), instance.entities...)
}

// Len reports how many of this instance's entities remain alive in its World.
func (instance *Instance) Len() int {
	if instance == nil || instance.world == nil {
		return 0
	}

	count := 0

	for _, entity := range instance.entities {
		if instance.world.Alive(entity) {
			count++
		}
	}

	return count
}

// Remove immediately removes all remaining entities as one logical instance.
// Shared source geometry and textures remain until explicit Model.Unload.
func (instance *Instance) Remove() bool {
	if instance == nil || instance.world == nil {
		return false
	}

	removed := false
	for _, entity := range instance.entities {
		removed = instance.world.Remove(entity) || removed
	}

	delete(instance.model.instances, instance)
	instance.world = nil
	instance.entities = nil

	return removed
}

func (model *Model) pruneInstances() {
	for instance := range model.instances {
		if instance.Len() == 0 {
			instance.world = nil
			instance.entities = nil
			delete(model.instances, instance)
		}
	}
}

func validRootTransform(transform engine.Transform) bool {
	values := [9]float32{transform.Position.X, transform.Position.Y, transform.Position.Z,
		transform.Rotation.X, transform.Rotation.Y, transform.Rotation.Z,
		transform.Scale.X, transform.Scale.Y, transform.Scale.Z}
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}

	if transform.Scale == (engine.Vector3{}) {
		return true
	}

	return float64(transform.Scale.X)*float64(transform.Scale.Y)*float64(transform.Scale.Z) > 0
}
