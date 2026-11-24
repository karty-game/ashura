//go:build !js

package assetbuild

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func marshalRecord(value any) ([]byte, error) { return json.Marshal(value) }

func imageResult(set ImageSet) (Result, error) {
	encoded, err := EncodeImageSet(set)
	if err != nil {
		return nil, err
	}

	return Result{imageDataSlot: encoded}, nil
}

func imageInput(ctx *Context, ref Ref) (ImageSet, error) {
	data, err := ctx.Input(ref)
	if err != nil {
		return ImageSet{}, err
	}

	return DecodeImageSet(data)
}

func transformImages(plan *Plan, name, revision string, options any, include []string, run func(ImageSet) (ImageSet, error)) error {
	for _, pattern := range include {
		if _, err := MatchAsset(pattern, ""); err != nil {
			return fmt.Errorf("asset selection %q: %w", pattern, err)
		}
	}

	for index, resource := range plan.Resources {
		selected, err := selectedImage(include, resource.ID)
		if err != nil {
			return err
		}

		if !selected {
			continue
		}

		applicable, schemaErr := imageResourceApplies(plan, resource.Ref, ImageSetSchema)
		if schemaErr != nil {
			return schemaErr
		}

		if !applicable {
			continue
		}

		refs, err := plan.Add(Action{
			Parallel: true,
			ID:       imageActionID(plan, name+"/"+resource.ID), Processor: name, Revision: revision, Options: options,
			Inputs: []Ref{resource.Ref}, Outputs: []Slot{{Name: imageDataSlot, Schema: ImageSetSchema}},
			Run: func(ctx *Context) (Result, error) {
				set, inputErr := imageInput(ctx, resource.Ref)
				if inputErr != nil {
					return nil, inputErr
				}

				set, inputErr = run(set)
				if inputErr != nil {
					return nil, inputErr
				}

				return imageResult(set)
			},
		})
		if err != nil {
			return err
		}

		plan.Resources[index].Ref = refs[0]
		plan.carryInspectedImages(resource.Ref, refs[0])
	}

	return nil
}

func requireResourceSchema(plan *Plan, ref Ref, schema string) error {
	for _, action := range plan.actions {
		if action.ID != ref.Action {
			continue
		}

		for _, slot := range action.Outputs {
			if slot.Name == ref.Slot && slot.Schema == schema {
				return nil
			}
		}
	}

	return fmt.Errorf("resource %s/%s requires schema %s: %w", ref.Action, ref.Slot, schema, errImageRecord)
}

// Image stages leave unrelated typed resources available for their own plugins.
// An image resource at the wrong processing stage remains an ordering error.
func imageResourceApplies(plan *Plan, ref Ref, expected string) (bool, error) {
	schema, err := plan.SlotSchema(ref)
	if err != nil {
		return false, err
	}

	if schema != ImageSetSchema && schema != PreparedSetSchema {
		return false, nil
	}

	if schema != expected {
		return false, fmt.Errorf("resource %s/%s requires schema %s: %w", ref.Action, ref.Slot, expected, errImageRecord)
	}

	return true, nil
}

func selectedImage(patterns []string, identity string) (bool, error) {
	if len(patterns) == 0 {
		return true, nil
	}

	for _, pattern := range patterns {
		matched, err := MatchAsset(pattern, identity)
		if err != nil {
			return false, fmt.Errorf("asset selection %q: %w", pattern, err)
		}

		if matched {
			return true, nil
		}
	}

	return false, nil
}

func imageActionID(plan *Plan, base string) string {
	id := base
	for suffix := 2; plan.ids[plan.scopedIdentity(id)]; suffix++ {
		id = base + "#" + strconv.Itoa(suffix)
	}

	return id
}
