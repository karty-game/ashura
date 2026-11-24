package level

import (
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
)

// PresentationRangesPerStep bounds newly visible prebuilt material ranges per
// PresentNext call. No new geometry ranges, chunks or sectors are constructed.
const PresentationRangesPerStep = 8

// Instantiate is the synchronous convenience form of BeginPresentation and
// PresentNext. Interactive streaming should present once per update instead.
func (chunk *LoadedChunk) Instantiate(world *engine.World) (*Instance, error) {
	instance, err := chunk.BeginPresentation(world)
	if err != nil {
		return nil, err
	}

	for {
		done, presentErr := instance.PresentNext()
		if presentErr != nil {
			return nil, presentErr
		}

		if done {
			return instance, nil
		}
	}
}

// BeginPresentation establishes presentation lifetime without adding draw
// records. Remove cancels partial presentation; pending instances retain their
// owning chunk even before their first range becomes visible.
func (chunk *LoadedChunk) BeginPresentation(world *engine.World) (*Instance, error) {
	if chunk == nil || chunk.closed || chunk.loader == nil || chunk.loader.closed || chunk.loader.app.Closed() {
		return nil, errLifetime
	}

	if world == nil || len(chunk.pages) == 0 {
		return nil, ErrFormat
	}

	instance := &Instance{chunk: chunk, world: world, presenting: true}
	chunk.instances[instance] = true

	return instance, nil
}

// PresentNext adds at most eight existing prepared material ranges from one
// page. It performs no geometry conversion, subdivision, repacking or resource
// fetch. Call once per App update to bound first-draw buffer creation. Failure
// removes the entire partial presentation; completed calls are idempotent.
func (instance *Instance) PresentNext() (bool, error) {
	if instance == nil || instance.chunk == nil || instance.world == nil {
		return true, errLifetime
	}

	chunk := instance.chunk
	if chunk.closed || chunk.loader.closed || chunk.loader.app.Closed() {
		instance.Remove()

		return true, errLifetime
	}

	if !instance.presenting {
		return true, nil
	}

	page := chunk.pages[instance.nextPage]

	count := min(PresentationRangesPerStep, page.materialCount()-instance.nextPart)
	if count <= 0 {
		instance.Remove()

		return true, leveldata.ErrFormat
	}

	transform := engine.DefaultTransform()
	transform.Position = engine.Vector3{X: chunk.description.Origin[0], Y: chunk.description.Origin[1], Z: chunk.description.Origin[2]}

	modelInstance, err := page.model.InstantiateRange(instance.world, transform, instance.nextPart, count)
	if err != nil {
		instance.Remove()

		return true, err
	}

	instance.models = append(instance.models, modelInstance)
	for offset, entity := range modelInstance.Entities() {
		if err = instance.bindPagePart(page, entity, instance.nextPart+offset); err != nil {
			instance.Remove()

			return true, err
		}
	}

	instance.nextPart += count
	if instance.nextPart == page.materialCount() {
		instance.nextPage++
		instance.nextPart = 0
	}

	if instance.nextPage == len(chunk.pages) {
		instance.presenting = false

		return true, nil
	}

	return false, nil
}

// Remove drops every partial or completed draw record and leaves shared chunk
// resources and logical scene objects available. Repeated calls are harmless.
func (instance *Instance) Remove() {
	if instance == nil || instance.chunk == nil {
		return
	}

	for _, modelInstance := range instance.models {
		modelInstance.Remove()
	}

	instance.decals.Remove()
	instance.decals = nil
	delete(instance.chunk.instances, instance)
	instance.models = nil
	instance.chunk = nil
	instance.world = nil
	instance.presenting = false
}

func (instance *Instance) bindPagePart(page loadedPage, entity engine.Entity, index int) error {
	if err := instance.bindDecalReceiver(entity); err != nil {
		return err
	}

	if len(page.standard) > 0 {
		return instance.world.Set(entity, page.standard[index])
	}

	engine.RemoveComponent[material.Material](instance.world, entity)

	return instance.world.Set(entity, page.materials[index])
}
