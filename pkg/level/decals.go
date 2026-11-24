package level

import (
	"github.com/karty-game/ashura/pkg/decal"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/texture"
)

// ChunkDecals enables decal receiving on a chunk's prepared material ranges
// and optionally mounts one offline projector batch alongside those ranges.
type ChunkDecals struct {
	Batch          *decal.Prepared
	Atlas          *texture.Texture
	ReceiverLayers uint32
}

// SetDecals sets decal resources before the first PresentNext call. The returned
// placement is removed with the chunk presentation. A nil batch allows
// gameplay marks on the chunk without attaching authored projectors.
func (instance *Instance) SetDecals(options ChunkDecals) (*decal.Placement, error) {
	if instance == nil || instance.world == nil || !instance.presenting {
		return nil, errLifetime
	}

	if len(instance.models) != 0 || instance.decals != nil || options.Batch == nil && options.Atlas != nil {
		return nil, ErrFormat
	}

	if options.Batch != nil {
		placement, err := instance.world.MountDecals(options.Batch, options.Atlas)
		if err != nil {
			return nil, err
		}

		instance.decals = placement
	}

	instance.decalLayers = options.ReceiverLayers

	return instance.decals, nil
}

// SetDecalEnabled updates one authored projector after its chunk is admitted.
func (state *Presentation) SetDecalEnabled(chunkID string, index int, enabled bool) error {
	if state == nil || state.Status().Closed {
		return errLifetime
	}

	for _, instance := range state.instances {
		if instance.chunk != nil && instance.chunk.description.ID == chunkID {
			return instance.decals.SetEnabled(index, enabled)
		}
	}

	return ErrFormat
}

func (instance *Instance) bindDecalReceiver(entity engine.Entity) error {
	if instance.decalLayers == 0 {
		return nil
	}

	return instance.world.Set(entity, decal.Receiver{Layers: instance.decalLayers})
}

func (state *Presentation) attachChunkDecals(instance *Instance) error {
	options, err := state.options.Decals(instance.chunk.description.ID)
	if err != nil {
		return err
	}

	_, err = instance.SetDecals(options)

	return err
}
