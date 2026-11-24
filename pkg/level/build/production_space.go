//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

type productionSpacePlan struct {
	scene    assetbuild.Ref
	geometry assetbuild.Ref
	chunks   []chunkRecord
	samples  []assetbuild.Ref
}

// planProductionSpaces joins immutable linear results inside a logical space
// before reconstruction. Runtime cells and receiver jobs cannot cut a filter's
// support. Unrelated overlapping spaces never join the same resolve action.
func (plugin *productionPlugin) planProductionSpaces(plan *assetbuild.Plan, identity string, record partitioned) ([]assetbuild.Ref, error) {
	groups := make(map[string]*productionSpacePlan)

	var ordered []*productionSpacePlan

	for _, chunk := range record.Chunks {
		key := chunk.Description.Space + "/" + chunk.Occluders.SHA256

		group := groups[key]
		if group == nil {
			scene, exists := plugin.earlyScenes[identity+"/"+key]
			if !exists {
				var err error

				scene, err = plugin.planScene(plan, identity, key, record, chunk)
				if err != nil {
					return nil, err
				}
			}

			group = &productionSpacePlan{scene: scene.lighting, geometry: scene.geometry}
			groups[key] = group
			ordered = append(ordered, group)
		}

		group.chunks = append(group.chunks, chunk)

		for _, page := range chunk.Pages {
			if page.Receivers.SHA256 == "" || page.UVLayout.SHA256 == "" {
				return nil, fmt.Errorf("production lightmapper requires sparse production UV inputs: %w", ErrInvalid)
			}
		}
	}

	resolved := make(map[string]assetbuild.Ref)
	for _, group := range ordered {
		if err := plugin.planSpaceResolve(plan, identity, group, resolved); err != nil {
			return nil, err
		}
	}

	chunks := make([]assetbuild.Ref, len(record.Chunks))
	for index, chunk := range record.Chunks {
		chunks[index] = resolved[chunk.Description.ID]
	}

	return chunks, nil
}

func (plugin *productionPlugin) planSpaceResolve(
	plan *assetbuild.Plan,
	identity string,
	group *productionSpacePlan,
	resolved map[string]assetbuild.Ref,
) error {
	combined := group.chunks[0]
	combined.Description.ID = "space/" + combined.Description.Space + "/" + combined.Occluders.SHA256

	combined.Pages = nil
	for _, chunk := range group.chunks {
		combined.Pages = append(combined.Pages, chunk.Pages...)
	}

	if len(combined.Pages) != 0 {
		if err := plugin.planBackend(plan, identity); err != nil {
			return err
		}
	}

	var (
		support assetbuild.Ref
		err     error
	)
	if plugin.options.Reconstruction != nil {
		support, err = plugin.planProductionSupport(plan, identity, combined, group.geometry)
		if err != nil {
			return err
		}
	}

	// Register topology admission before tracing actions.
	group.samples, err = plugin.planSpaceSamples(plan, identity, group, combined)
	if err != nil {
		return err
	}

	ref, err := plugin.planChunkResolve(plan, identity, combined, group.scene, group.samples, support)
	if err != nil {
		return err
	}

	return planProductionSlices(plan, identity, group, resolved, ref)
}

func planProductionSlices(
	plan *assetbuild.Plan,
	identity string,
	group *productionSpacePlan,
	resolved map[string]assetbuild.Ref,
	ref assetbuild.Ref,
) error {
	start := 0
	for _, chunk := range group.chunks {
		low, high := start, start+len(chunk.Pages)

		refs, err := plan.Add(assetbuild.Action{
			ID:        "level/production/split/" + identity + "/" + chunk.Description.ID,
			Processor: "lightmapper-space-split", Revision: "original-chunks@1",
			Inputs: []assetbuild.Ref{ref}, Options: [2]int{low, high},
			Outputs: []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var pages []pageRecord
				if err := readRecord(ctx, ref, &pages); err != nil {
					return nil, err
				}

				if low < 0 || high > len(pages) || low > high {
					return nil, ErrInvalid
				}

				return result(pages[low:high])
			},
		})
		if err != nil {
			return err
		}

		resolved[chunk.Description.ID] = refs[0]
		start = high
	}

	return nil
}
