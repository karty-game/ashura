package engine

import "github.com/karty-game/ashura/internal/render"

// The outer source map owns geometry identity; this alias retains inline keys.
type materialKey = render.MaterialKey

type sourceBatches struct {
	key      sourceKey
	firstKey materialKey
	first    *frameBatch
	others   map[materialKey]*frameBatch
	used     bool
	active   bool
}

// Inactive groups may still have a bounded entity cache pointing at them. Drop
// every source/material/draw reference when their registry entry is retired.
func (groups *sourceBatches) retire() {
	groups.key = sourceKey{}
	groups.firstKey = materialKey{}
	groups.first, groups.others = nil, nil
	groups.used, groups.active = false, false
}

func batchMaterialKey(draw render.DrawCall) materialKey { return render.EffectiveMaterial(draw) }

func (groups *sourceBatches) find(draw render.DrawCall) *frameBatch {
	if groups.first == nil {
		return nil
	}

	key := batchMaterialKey(draw)
	if groups.firstKey == key {
		return groups.first
	}

	return groups.others[key]
}

func (groups *sourceBatches) add(batch *frameBatch) {
	if groups.first == nil {
		groups.first = batch
		groups.firstKey = batchMaterialKey(batch.draw)

		return
	}

	if groups.others == nil {
		groups.others = make(map[materialKey]*frameBatch)
	}

	groups.others[batchMaterialKey(batch.draw)] = batch
}
