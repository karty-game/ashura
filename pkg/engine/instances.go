package engine

import "github.com/karty-game/karty-webgpu/internal/render"

type frameBatch struct {
	draw            render.DrawCall
	seen            bool
	transform       Transform
	matrix          [16]float32
	matrixReady     bool
	billboard       billboardSnapshot
	billboardActive bool
	members         []uint32
	changed         []int
	cursor          int
	version         uint64
	dirty           bool
}

// Keep probe identity inline instead of boxing a composite value in InstanceKey.
type sourceKey struct {
	mesh  any
	probe uint32
}

type frameStorage struct {
	frame           render.Frame
	batches         map[uint32]*frameBatch
	groups          map[sourceKey]*sourceBatches
	order           []*frameBatch
	camera          Entity
	cameraTransform Transform
	projection      projectionSettings
	aspect          float32
	cameraReady     bool
	probes          []reflectionProbe
}

func (storage *frameStorage) reset() {
	for _, batch := range storage.order {
		batch.seen = false
	}

	for _, groups := range storage.groups {
		groups.first = nil
		groups.used = false
		clear(groups.others)
	}

	clear(storage.order)
	storage.order = storage.order[:0]
	clear(storage.frame.Draws)
	storage.frame.Draws = storage.frame.Draws[:0]
	storage.frame.Lights = storage.frame.Lights[:0]
	storage.frame.ClearColor = [4]float32{0.08, 0.09, 0.12, 1}
	storage.frame.PointerLock = false
	storage.probes = storage.probes[:0]
}

func (storage *frameStorage) batch(identifier uint32) *frameBatch {
	if storage.batches == nil {
		storage.batches = make(map[uint32]*frameBatch)
		storage.groups = make(map[sourceKey]*sourceBatches)
	}

	if cached := storage.batches[identifier]; cached != nil {
		return cached
	}

	cached := &frameBatch{}
	storage.batches[identifier] = cached

	return cached
}

// Structural mutations retire stale asset references without discarding reusable
// matrix/instance storage. Non-renderable entities no longer need a draw cache.
func (storage *frameStorage) invalidate(entity uint32, renderable bool) {
	if batch := storage.batches[entity]; batch != nil {
		batch.draw = render.DrawCall{Transform: batch.draw.Transform, Instances: batch.draw.Instances}

		if !renderable {
			delete(storage.batches, entity)
		}
	}
}

func (storage *frameStorage) append(draw render.DrawCall) {
	var group *sourceBatches

	if draw.InstanceKey != nil {
		key := sourceKey{mesh: draw.InstanceKey, probe: draw.ProbeID}

		group = storage.groups[key]
		if group == nil {
			group = &sourceBatches{}
			storage.groups[key] = group
		}

		group.used = true
		if existing := group.find(draw); existing != nil {
			existing.addInstance(draw)

			return
		}
	}

	batch := storage.batch(draw.ID)
	previous := batch.draw
	batch.changed = batch.changed[:0]

	batch.draw = draw
	if draw.InstanceKey != nil {
		batch.draw.Instances = previous.Instances
		batch.cursor = 0
		batch.dirty = batch.version == 0
		batch.addInstance(draw)
		group.add(batch)
	} else if batch.version == 0 || previous.Transform != draw.Transform || len(previous.Instances) != 0 {
		batch.version++
		batch.changed = append(batch.changed, 0)
	}

	batch.seen = true
	storage.order = append(storage.order, batch)
}

func (batch *frameBatch) addInstance(draw render.DrawCall) {
	index := batch.cursor
	if index >= len(batch.draw.Instances) {
		batch.draw.Instances = append(batch.draw.Instances, draw.Instance)
		batch.members = append(batch.members, draw.ID)
		batch.changed = append(batch.changed, index)
		batch.dirty = true
	} else if batch.draw.Instances[index] != draw.Instance || batch.members[index] != draw.ID {
		batch.draw.Instances[index], batch.members[index] = draw.Instance, draw.ID
		batch.changed = append(batch.changed, index)
		batch.dirty = true
	}

	batch.cursor++
}

func (storage *frameStorage) finish() render.Frame {
	for key, groups := range storage.groups {
		if !groups.used {
			delete(storage.groups, key)
		}
	}

	for _, batch := range storage.order {
		if batch.draw.InstanceKey != nil {
			if len(batch.draw.Instances) != batch.cursor {
				batch.dirty = true
			}

			batch.draw.Instances = batch.draw.Instances[:batch.cursor]

			batch.members = batch.members[:batch.cursor]
			if batch.dirty {
				batch.version++
			}
		}

		batch.draw.InstanceVersion, batch.draw.InstanceOwner, batch.draw.ChangedInstances = batch.version, batch, batch.changed
		storage.frame.Draws = append(storage.frame.Draws, batch.draw)
	}

	return storage.frame
}
