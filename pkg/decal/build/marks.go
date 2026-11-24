//go:build !js

// Package decalbuild prepares authored projectors and bakes permanent albedo marks.
package decalbuild

import (
	"fmt"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/engine"
)

// Mark is an authored projector. Image names the input image resource for Bake,
// or the source name of an atlas region for Projectors.
type Mark struct {
	Image                  string
	Position, Normal, Size engine.Vector3
	Tint                   engine.Color
	Angle                  float32
	ReceiverMask           uint32
	Order                  int32
}

func (mark Mark) placement(minimum, maximum [2]float32) decaldata.Placement {
	return decaldata.Placement{
		Position: vector(mark.Position), Normal: vector(mark.Normal), Size: vector(mark.Size),
		Min: minimum, Max: maximum,
		Tint:  [4]float32{mark.Tint.R, mark.Tint.G, mark.Tint.B, mark.Tint.A},
		Angle: mark.Angle, Layers: mark.ReceiverMask, Order: mark.Order,
	}
}

func vector(value engine.Vector3) decaldata.Vector {
	return decaldata.Vector{value.X, value.Y, value.Z}
}

func imageReference(plan *assetbuild.Plan, name string) (int, assetbuild.Ref, error) {
	for index, resource := range plan.Resources {
		if resource.ID != name {
			continue
		}

		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return 0, assetbuild.Ref{}, err
		}

		if schema == assetbuild.ImageSetSchema {
			return index, resource.Ref, nil
		}

		break
	}

	return 0, assetbuild.Ref{}, fmt.Errorf("decal build needs image resource %q: %w", name, decaldata.ErrProjector)
}

func readImages(ctx *assetbuild.Context, resource assetbuild.Ref) (assetbuild.ImageSet, error) {
	data, err := ctx.Input(resource)
	if err != nil {
		return assetbuild.ImageSet{}, err
	}

	return assetbuild.DecodeImageSet(data)
}
