//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/meshopt"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// The level module owns its compound UV records. The independent mesh stage
// invokes this planning adapter without importing the baker or its record types.
func processUVGeometry(plan *assetbuild.Plan, resource assetbuild.Resource, operation string) (assetbuild.Ref, error) {
	schema, err := plan.SlotSchema(resource.Ref)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	if operation != "mesh-optimize" || schema != uvSchema {
		return assetbuild.Ref{}, fmt.Errorf(
			"geometry optimization must follow UV generation and precede baking; cleanup must precede UVs: %w",
			ErrInvalid,
		)
	}

	identity := "level/mesh-optimize/" + resource.ID
	err = plan.Expand(
		assetbuild.Expansion{
			ID:     identity + "/discover",
			Inputs: []assetbuild.Ref{resource.Ref},
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				var record partitioned
				if err := readRecord(ctx, resource.Ref, &record); err != nil {
					return err
				}

				var pages []assetbuild.Ref

				for chunkIndex, chunk := range record.Chunks {
					for pageIndex, page := range chunk.Pages {
						ref, err := planGeometryPage(child, fmt.Sprintf("%s/%d/%d", identity, chunkIndex, pageIndex), page)
						if err != nil {
							return err
						}

						pages = append(pages, ref)
					}
				}

				return planGeometryIndex(child, identity, resource, pages)
			},
		},
	)
	// Discovery installs the replacement before the next recipe stage runs.
	return resource.Ref, err
}

func planGeometryPage(plan *assetbuild.Plan, identity string, page pageRecord) (assetbuild.Ref, error) {
	refs, err := plan.Add(assetbuild.Action{
		Parallel: true, ID: identity, Processor: "level-mesh-optimize", Revision: meshopt.Identity + "/uv@1", Options: page,
		Artifacts: geometryPageArtifacts(page), Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.level-optimized-page@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			output, err := optimizeUVPage(ctx, page)
			if err != nil {
				return nil, err
			}

			return result(output)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func planGeometryIndex(plan *assetbuild.Plan, identity string, resource assetbuild.Resource, pages []assetbuild.Ref) error {
	refs, err := plan.Add(assetbuild.Action{ID: identity, Processor: "level-mesh-optimize-index", Revision: "ordered-pages@1",
		Inputs: append([]assetbuild.Ref{resource.Ref}, pages...), Outputs: []assetbuild.Slot{{Name: slot, Schema: uvSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			var current partitioned
			if err := readRecord(ctx, resource.Ref, &current); err != nil {
				return nil, err
			}

			offset := 0

			for chunkIndex := range current.Chunks {
				for pageIndex := range current.Chunks[chunkIndex].Pages {
					if offset >= len(pages) {
						return nil, ErrInvalid
					}

					if err := readRecord(ctx, pages[offset], &current.Chunks[chunkIndex].Pages[pageIndex]); err != nil {
						return nil, err
					}

					offset++
				}
			}

			if offset != len(pages) {
				return nil, ErrInvalid
			}

			return result(current)
		},
	})
	if err == nil {
		replaceResource(plan, resource.ID, refs[0])
	}

	return err
}

func geometryPageArtifacts(page pageRecord) []assetbuild.Artifact {
	var artifacts []assetbuild.Artifact

	for _, ref := range []assetbuild.Artifact{page.Geometry, page.UVLayout, page.BakeInputs, page.Receivers} {
		if ref.Size > 0 {
			artifacts = append(artifacts, ref)
		}
	}

	return artifacts
}
