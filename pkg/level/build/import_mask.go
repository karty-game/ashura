//go:build !js

package levelbuild

import (
	geometry "github.com/karty-game/karty-webgpu/internal/levelbuild"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func separateMaskedPlacement(
	ctx *assetbuild.Context,
	entry placement,
	enabled bool,
	prepared assetbuild.Ref,
) (*placement, *placement, error) {
	if !enabled || entry.Dynamic {
		return &entry, nil, nil
	}

	data, err := ctx.Input(prepared)
	if err != nil {
		return nil, nil, err
	}

	document, err := modeldata.Decode(data)
	if err != nil {
		return nil, nil, err
	}

	opaque, masked := selectModelParts(document, false), selectModelParts(document, true)
	if len(masked.Parts) == 0 {
		return &entry, nil, nil
	}

	derived := entry
	derived.ID += "/#masked"
	derived.Model += "#masked"
	derived.Dynamic = true
	derived.SSBump = nil
	masked.Source = derived.Model

	masked, err = geometry.PrepareModel(masked, entry.Transform)
	if err != nil {
		return nil, nil, err
	}

	derived.Transform = [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

	derived.Asset, err = storeImportedModel(ctx, masked)
	if err != nil {
		return nil, nil, err
	}

	derived.Model += "/" + derived.Asset.SHA256
	if len(opaque.Parts) == 0 {
		return nil, &derived, nil
	}

	entry.Asset, err = storeImportedModel(ctx, opaque)
	if err != nil {
		return nil, nil, err
	}

	return &entry, &derived, nil
}

// Each subset retains only referenced textures, with exact bytes and material
// flags. A mask texture must never enlarge an opaque chunk or become opaque.
func selectModelParts(input modeldata.Document, masked bool) modeldata.Document {
	output := modeldata.Document{Source: input.Source}
	textures := make(map[int32]int32)
	remap := func(index int32) int32 {
		if index < 0 {
			return index
		}

		if selected, ok := textures[index]; ok {
			return selected
		}
		//nolint:gosec // G115: the decoded source limits distinct texture slots to modeldata.MaxTextures (64).
		selected := int32(len(output.Textures))
		textures[index] = selected
		output.Textures = append(output.Textures, input.Textures[index])

		return selected
	}

	for _, part := range input.Parts {
		if part.Material.AlphaMask != masked {
			continue
		}

		part.Material.Albedo = remap(part.Material.Albedo)
		part.Material.Normal = remap(part.Material.Normal)
		part.Material.AO = remap(part.Material.AO)
		output.Parts = append(output.Parts, part)
	}

	return output
}

func storeImportedModel(ctx *assetbuild.Context, document modeldata.Document) (assetbuild.Artifact, error) {
	data, err := modeldata.Encode(document)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	return ctx.StoreArtifact(data)
}
