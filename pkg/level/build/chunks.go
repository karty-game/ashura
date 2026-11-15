//go:build !js

package levelbuild

import (
	"fmt"
	"slices"

	geometry "github.com/karty-game/karty-webgpu/internal/levelbuild"
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// StaticChunks partitions static placements without changing logical scene factories.
func StaticChunks(options ChunkOptions) assetbuild.Plugin {
	return stage{name: "chunks", schema: importSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(
			assetbuild.Expansion{
				ID:     "level/chunks/" + resource.ID,
				Inputs: []assetbuild.Ref{resource.Ref},
				Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
					return discoverChunks(ctx, child, resource, options)
				},
			},
		)
	}}
}

func partition(ctx *assetbuild.Context, source imported, options ChunkOptions) (assetbuild.Result, error) {
	placements, err := partitionPlacements(ctx, source)
	if err != nil {
		return nil, err
	}

	split, err := geometry.Split(
		ctx,
		placements,
		geometry.Options{Size: options.Size, Origin: options.Origin, MaxChunks: options.MaxChunks, MaxTriangles: options.MaxTriangles},
	)
	if err != nil {
		return nil, err
	}

	size := options.Size
	if size == ([3]float32{}) {
		size = [3]float32{32, 32, 32}
	}

	output := partitioned{Source: source.Source, Size: size}
	for _, placement := range source.Placements {
		if placement.Dynamic {
			output.Dynamic = append(output.Dynamic, placement)
		}
	}

	spaces, err := storeVisibilitySpaces(ctx, split.Spaces)
	if err != nil {
		return nil, err
	}

	byID := map[string]placement{}
	for _, entry := range source.Placements {
		byID[entry.ID] = entry
	}

	for _, chunk := range split.Chunks {
		data, err := modeldata.Encode(chunk.Model)
		if err != nil {
			return nil, err
		}

		artifact, err := ctx.StoreArtifact(data)
		if err != nil {
			return nil, err
		}

		record := chunkRecord{
			Geometry:  artifact,
			Occluders: spaces[chunk.Space],
			SSBump:    map[string]assetbuild.Artifact{},
			Description: leveldata.Chunk{
				ID:     chunk.ID,
				Space:  chunk.Space,
				Cell:   chunk.Cell,
				Origin: chunk.Origin,
				Min:    chunk.Min,
				Max:    chunk.Max,
			},
		}
		if err := chunkSourceMaterials(chunk, byID, &record); err != nil {
			return nil, err
		}

		output.Chunks = append(output.Chunks, record)
	}

	return result(output)
}

func chunkSourceMaterials(chunk geometry.Chunk, byID map[string]placement, record *chunkRecord) error {
	for _, sourceRange := range chunk.Sources {
		if !slices.Contains(record.Description.Sources, sourceRange.ID) {
			record.Description.Sources = append(record.Description.Sources, sourceRange.ID)
		}

		override, ok := byID[sourceRange.ID].SSBump[sourceRange.Name]
		if !ok {
			continue
		}

		name := chunk.Model.Parts[sourceRange.Part].Name
		if old, ok := record.SSBump[name]; ok && old != override {
			return fmt.Errorf("incompatible merged Source material: %w", ErrInvalid)
		}

		record.SSBump[name] = override
	}

	return nil
}

func discoverChunks(ctx *assetbuild.Context, child *assetbuild.Plan, resource assetbuild.Resource, options ChunkOptions) error {
	var initial imported
	if err := readRecord(ctx, resource.Ref, &initial); err != nil {
		return err
	}

	var artifacts []assetbuild.Artifact
	for _, placement := range initial.Placements {
		artifacts = append(artifacts, placement.Asset)
		for _, value := range placement.SSBump {
			artifacts = append(artifacts, value)
		}
	}

	refs, err := child.Add(
		assetbuild.Action{
			ID:        "level/partition/" + resource.ID,
			Processor: "level-chunks",
			Revision:  "grid-clip-material@2",
			Artifacts: artifacts,
			Options: struct {
				Options    ChunkOptions
				Placements []placement
			}{options, initial.Placements},
			Outputs: []assetbuild.Slot{{Name: slot, Schema: chunksSchema}},
			Run:     func(ctx *assetbuild.Context) (assetbuild.Result, error) { return partition(ctx, initial, options) },
		},
	)
	if err != nil {
		return err
	}

	combined, err := child.Add(
		assetbuild.Action{
			ID:        "level/chunk-index/" + resource.ID,
			Processor: "level-chunk-index",
			Revision:  "logical-roster@1",
			Inputs:    []assetbuild.Ref{resource.Ref, refs[0]},
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: chunksSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var (
					source   imported
					prepared partitioned
				)

				if err := readRecord(ctx, resource.Ref, &source); err != nil {
					return nil, err
				}

				if err := readRecord(ctx, refs[0], &prepared); err != nil {
					return nil, err
				}

				prepared.Objects = source.Objects
				prepared.Points = source.Points
				prepared.SkyColor = source.SkyColor
				prepared.GroundColor = source.GroundColor

				return result(prepared)
			},
		},
	)
	if err != nil {
		return err
	}

	replaceResource(child, resource.ID, combined[0])

	return nil
}

func partitionPlacements(ctx *assetbuild.Context, source imported) ([]geometry.Placement, error) {
	placements := make([]geometry.Placement, 0, len(source.Placements))
	for _, placement := range source.Placements {
		if placement.Dynamic {
			continue
		}

		data, err := ctx.ReadArtifact(placement.Asset)
		if err != nil {
			return nil, err
		}

		doc, err := modeldata.Decode(data)
		if err != nil {
			return nil, err
		}

		for _, part := range doc.Parts {
			if part.Material.AlphaMask {
				return nil, fmt.Errorf(
					"static placement %q model %q part %q uses alpha MASK; keep it in an independent model with render3d.model.isDynamic: true: %w",
					placement.ID,
					placement.Model,
					part.Name,
					ErrInvalid,
				)
			}
		}

		groups := map[string]string{}
		for name, value := range placement.SSBump {
			groups[name] = value.SHA256
		}

		placements = append(
			placements,
			geometry.Placement{ID: placement.ID, Space: placement.Space, Model: doc, Transform: placement.Transform, PartGroups: groups},
		)
	}

	return placements, nil
}

func storeVisibilitySpaces(ctx *assetbuild.Context, input []geometry.SpaceScene) (map[string]assetbuild.Artifact, error) {
	spaces := map[string]assetbuild.Artifact{}

	for _, space := range input {
		data, err := modeldata.Encode(space.Model)
		if err != nil {
			return nil, err
		}

		artifact, err := ctx.StoreArtifact(data)
		if err != nil {
			return nil, err
		}

		spaces[space.ID] = artifact
	}

	return spaces, nil
}
