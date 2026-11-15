//go:build !js

package assetbuild

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type catalogPlugin struct{}

// GoCatalog emits a typed assets package for KTX2 resources. Atlas pages have
// page declarations and source-region metadata, never misleading whole-page
// declarations under the original source image names.
func GoCatalog() Plugin { return catalogPlugin{} }

func (catalogPlugin) Name() string { return "go-catalog" }

func (catalogPlugin) Plan(plan *Plan) error {
	inputs := make([]Ref, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		applicable, schemaErr := imageResourceApplies(plan, resource.Ref, PreparedSetSchema)
		if schemaErr != nil {
			return schemaErr
		}

		if !applicable {
			continue
		}

		ref := resource.Ref

		ref.Slot = imageCatalogSlot
		if err := requireResourceSchema(plan, ref, "karty.image-catalog@1"); err != nil {
			return err
		}

		inputs = append(inputs, ref)
	}

	refs, err := plan.Add(Action{
		Parallel: true,
		ID:       imageActionID(plan, "go-catalog"), Processor: "go-catalog", Revision: "named-assets@1",
		Inputs: inputs, Outputs: []Slot{{Name: imageFilesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *Context) (Result, error) {
			set, inputErr := readCatalogInputs(ctx, inputs)
			if inputErr != nil {
				return nil, inputErr
			}

			code, codeErr := catalogCode(set)
			if codeErr != nil {
				return nil, codeErr
			}

			files := Bundle{"assets.go": code}

			if len(set.Regions) > 0 {
				mapping, mappingErr := json.MarshalIndent(set.Regions, "", "  ")
				if mappingErr != nil {
					return nil, mappingErr
				}

				files["atlas.json"] = append(mapping, '\n')
			}

			data, marshalErr := json.Marshal(files)
			if marshalErr != nil {
				return nil, marshalErr
			}

			return Result{imageFilesSlot: data}, nil
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func readCatalogInputs(ctx *Context, refs []Ref) (PreparedSet, error) {
	var result PreparedSet

	for _, ref := range refs {
		data, err := ctx.Input(ref)
		if err != nil {
			return PreparedSet{}, err
		}

		var set PreparedSet
		if err := json.Unmarshal(data, &set); err != nil {
			return PreparedSet{}, fmt.Errorf("catalog input: %w", err)
		}

		result.Images = append(result.Images, set.Images...)
		result.Regions = append(result.Regions, set.Regions...)
	}

	slices.SortFunc(result.Images, func(left, right PreparedImage) int { return strings.Compare(left.Name, right.Name) })
	slices.SortFunc(result.Regions, func(left, right AtlasRegion) int { return strings.Compare(left.Name, right.Name) })

	return result, nil
}
