package engine

import (
	"cmp"
	"errors"
	"slices"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

// MaxDecalProjectors bounds the projector components in one World.
const MaxDecalProjectors = render.MaxDecals

var (
	errDecalLimit      = errors.New("engine: decal projector limit exceeded")
	errDecalAtlas      = errors.New("engine: decals in one world require a shared prepared sRGB alpha atlas")
	errDecalProjection = errors.New("engine: invalid decal projector dimensions, placement, tint or image region")
)

// DecalImage references a region of an immutable, App-owned sRGB alpha texture.
// The zero UV rectangle selects the full texture. Atlas regions use normalized
// top-left coordinates and require offline padding for their prepared mips.
type DecalImage struct {
	Texture      *texture.Texture
	UVMin, UVMax Vector2
}

// DecalProjector projects an image into a box centered at Position. The normal
// points out of the receiving surface. Size is width, height and total depth.
// Angle is a right-handed rotation around Normal in radians. Tint multiplies
// linear RGB and source alpha. Higher Order values blend later.
type DecalProjector struct {
	Image                  DecalImage
	Position, Normal, Size Vector3
	Tint                   Color
	Angle                  float32
	ReceiverMask           uint32
	Order                  int
}

// DecalReceiver opts an entity into Standard or Source decal shading. A
// projector affects it when Layers and ReceiverMask share one or more bits.
type DecalReceiver struct {
	Layers uint32
}

type decalBinding struct {
	entity   Entity
	source   *DecalProjector
	previous DecalProjector
	record   [render.DecalRecordBytes]byte
	valid    bool
}

type worldDecals struct {
	bindings  []decalBinding
	frame     render.DecalFrame
	static    *staticDecals
	liveAtlas *texture.Texture
	dirty     bool
}

func (entry *entityBindings) bindDecals(component any) {
	switch value := component.(type) {
	case *DecalProjector:
		entry.decalProjector = value
	case *DecalReceiver:
		entry.decalReceiver = value
	}
}

func (world *World) bindDecal(entity Entity, source *DecalProjector) {
	if world.decals == nil {
		if source == nil {
			return
		}

		world.decals = &worldDecals{}
	}

	state := world.decals
	for index := range state.bindings {
		binding := &state.bindings[index]
		if binding.entity != entity {
			continue
		}

		if source == nil {
			state.bindings = slices.Delete(state.bindings, index, index+1)
			state.dirty = true
		} else {
			binding.source = source
		}

		return
	}

	if source != nil {
		state.bindings = append(state.bindings, decalBinding{entity: entity, source: source})
		state.dirty = true
	}
}

func (state *worldDecals) snapshot() error {
	if len(state.bindings) > MaxDecalProjectors {
		return errDecalLimit
	}

	for index := range state.bindings {
		binding := &state.bindings[index]

		value := *binding.source
		if binding.valid && value == binding.previous {
			continue
		}

		record, err := packDecal(value)
		if err != nil {
			return err
		}

		binding.previous, binding.record, binding.valid = value, record, true
		state.dirty = true
	}

	if state.dirty {
		return state.rebuild()
	}

	return nil
}

func (state *worldDecals) rebuild() error {
	var atlas *texture.Texture

	for index := range state.bindings {
		value := state.bindings[index].previous
		if value.ReceiverMask == 0 || value.Tint.A == 0 {
			continue
		}

		if atlas != nil && atlas != value.Image.Texture {
			return errDecalAtlas
		}

		atlas = value.Image.Texture
	}

	slices.SortFunc(state.bindings, func(a, b decalBinding) int {
		if order := cmp.Compare(a.previous.Order, b.previous.Order); order != 0 {
			return order
		}

		return cmp.Compare(a.entity.index, b.entity.index)
	})

	count := 0

	for index := range state.bindings {
		binding := &state.bindings[index]
		if binding.previous.ReceiverMask == 0 || binding.previous.Tint.A == 0 {
			continue
		}

		state.frame.Records[count] = binding.record
		count++
	}

	state.liveAtlas, state.frame.Count = atlas, count
	state.frame.Atlas = atlas
	state.frame.Revision++
	state.dirty = false

	return nil
}

func packDecal(value DecalProjector) ([render.DecalRecordBytes]byte, error) {
	var empty [render.DecalRecordBytes]byte
	if value.ReceiverMask == 0 || value.Tint.A == 0 {
		return empty, nil
	}

	if !validDecalAtlas(value.Image.Texture) {
		return empty, errDecalAtlas
	}

	start, end := value.Image.UVMin, value.Image.UVMax
	if start == (Vector2{}) && end == (Vector2{}) {
		end = Vector2{X: 1, Y: 1}
	}

	projection, err := (decaldata.Placement{
		Position: vectorArray(value.Position), Normal: vectorArray(value.Normal), Size: vectorArray(value.Size),
		Min: [2]float32{start.X, start.Y}, Max: [2]float32{end.X, end.Y},
		Tint:  [4]float32{value.Tint.R, value.Tint.G, value.Tint.B, value.Tint.A},
		Angle: value.Angle, Layers: value.ReceiverMask,
	}).Project()
	if err != nil {
		return empty, errDecalProjection
	}

	return projection.Record(), nil
}

func validDecalAtlas(image *texture.Texture) bool {
	return texturedata.Valid(image) && texturedata.Space(image) == texturedata.SRGB && !texturedata.IsStorage(image) &&
		texturedata.Encoding(
			image,
		) != texture.BC1 && texturedata.Encoding(image) != texture.ETC2RGB && texturedata.Encoding(image) != texture.BC6H
}
