//go:build !js

// Package imagecatalog collects image records for offline page publishers.
package imagecatalog

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

// Inputs selects image sets and reports when KTX2 has already consumed them.
func Inputs(plan *assetbuild.Plan, publisher string, invalid error) ([]assetbuild.Ref, error) {
	inputs := make([]assetbuild.Ref, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return nil, err
		}

		if schema == assetbuild.PreparedSetSchema {
			return nil, fmt.Errorf("%s must consume images before KTX2: %w", publisher, invalid)
		}

		if schema == assetbuild.ImageSetSchema {
			inputs = append(inputs, resource.Ref)
		}
	}

	return inputs, nil
}

// Read combines the image sets in canonical name order for catalog generation.
func Read(ctx *assetbuild.Context, inputs []assetbuild.Ref) (assetbuild.ImageSet, error) {
	var result assetbuild.ImageSet

	for _, input := range inputs {
		data, err := ctx.Input(input)
		if err != nil {
			return assetbuild.ImageSet{}, err
		}

		set, err := assetbuild.DecodeImageSet(data)
		if err != nil {
			return assetbuild.ImageSet{}, err
		}

		result.Images = append(result.Images, set.Images...)
		result.Regions = append(result.Regions, set.Regions...)
	}

	slices.SortFunc(result.Images, func(a, b assetbuild.Image) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(result.Regions, func(a, b assetbuild.AtlasRegion) int { return strings.Compare(a.Name, b.Name) })

	return result, nil
}

// AddAtlasMetadata includes the region map when the catalog has packed images.
func AddAtlasMetadata(files assetbuild.Bundle, regions []assetbuild.AtlasRegion) error {
	if len(regions) == 0 {
		return nil
	}

	metadata, err := json.MarshalIndent(regions, "", "  ")
	if err != nil {
		return err
	}

	files["atlas.json"] = append(metadata, '\n')

	return nil
}
