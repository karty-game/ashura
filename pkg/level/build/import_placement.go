//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
)

func (state *levelImportState) importPlacement(
	ctx *assetbuild.Context,
	node scenebuild.ResolvedNode,
	transform [16]float32,
) (placement, error) {
	config := node.Components["render3d.model"]
	source, _ := config["asset"].(string)
	dynamic, _ := config["isDynamic"].(bool)

	artifact, err := state.modelArtifact(ctx, source)
	if err != nil {
		return placement{}, err
	}

	entry := placement{
		ID:        node.Path,
		Space:     nodeSpace(node.Path, state.options.Spaces),
		Model:     source,
		Transform: transform,
		Dynamic:   dynamic,
		Asset:     artifact,
	}
	if state.sourceOnly {
		return entry, nil
	}

	overrides, err := state.overrides(ctx, source)
	if err != nil {
		return placement{}, err
	}

	entry.SSBump = overrides
	if !dynamic {
		entry.SSBump, err = state.staticRelief(ctx, source, overrides)
		if err != nil {
			return placement{}, err
		}
	}

	return entry, nil
}

func (state *levelImportState) staticRelief(
	ctx *assetbuild.Context,
	source string,
	overrides map[string]assetbuild.Artifact,
) (map[string]assetbuild.Artifact, error) {
	if cached, ok := state.materials[source]; ok {
		return cached, nil
	}

	document := state.documents[source]

	selected := overrides
	if state.options.SeparateMasked {
		if err := validateSourceOverrides(document, overrides); err != nil {
			return nil, err
		}

		document = selectModelParts(document, false)
		selected = make(map[string]assetbuild.Artifact)

		for _, part := range document.Parts {
			if value, ok := overrides[part.Name]; ok {
				selected[part.Name] = value
			}
		}
	}

	cached, err := autoSSBump(ctx, document, selected)
	if err != nil {
		return nil, err
	}

	state.materials[source] = cached
	delete(state.documents, source)

	return cached, nil
}

func (state *levelImportState) importNodeModel(
	ctx *assetbuild.Context,
	node scenebuild.ResolvedNode,
	transform [16]float32,
	object *leveldata.Object,
	record *imported,
	nodes map[string]scenebuild.ResolvedNode,
) error {
	entry, err := state.importPlacement(ctx, node, transform)
	if err != nil {
		return err
	}

	opaque, masked, err := separateMaskedPlacement(ctx, entry, state.options.SeparateMasked, state.resources[entry.Model].Ref)
	if err != nil {
		return err
	}

	if opaque != nil {
		record.Placements = append(record.Placements, *opaque)
	}

	if masked != nil {
		if _, exists := nodes[masked.ID]; exists {
			return fmt.Errorf("derived masked object identity %q conflicts with an authored node: %w", masked.ID, ErrInvalid)
		}

		record.Placements = append(record.Placements, *masked)
		record.Objects = append(
			record.Objects,
			leveldata.Object{ID: masked.ID, Model: masked.Model, Dynamic: true, Transform: masked.Transform},
		)
	}

	object.Model = entry.Model
	object.Dynamic = entry.Dynamic

	return nil
}
