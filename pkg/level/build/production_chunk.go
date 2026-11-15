//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

// reconstructChunk gathers complete finalized page estimates before constructing
// surface support. UV jobs and atlas/page boundaries cannot truncate a filter's
// neighbors. GPU Reconstruct tiles interiors with immutable complete halos.
//
//nolint:cyclop,funlen,gocognit // Validate provenance, ownership and ordering across all immutable page results before halo reconstruction.
func (plugin *productionPlugin) reconstructChunk(
	ctx *assetbuild.Context,
	chunk chunkRecord,
	scene *bake.PreparedScene,
	samples []assetbuild.Ref,
	support assetbuild.Ref,
) ([]bake.ReceiverSet, []bake.Result, error) {
	if scene == nil || len(samples) != len(chunk.Pages) {
		return nil, nil, ErrInvalid
	}

	sets, results, err := plugin.readProductionPages(ctx, scene, chunk.Pages, samples)
	if err != nil {
		return nil, nil, err
	}

	var (
		receivers []bake.Receiver
		estimates []bake.Estimate
	)

	offsets := make([]int, len(chunk.Pages)+1)
	identities := make(map[uint64]bool)

	for index := range chunk.Pages {
		if results[index].SceneHash != scene.Hash() || len(results[index].Estimates) != len(sets[index].Receivers) {
			return nil, nil, fmt.Errorf("production page %d provenance/count mismatch: %w", index, ErrInvalid)
		}

		if index > 0 {
			first, current := results[0].Options, results[index].Options
			if first.Outputs != current.Outputs || first.Bounces != current.Bounces || first.RouletteDepth != current.RouletteDepth ||
				first.ExactLights != current.ExactLights {
				return nil, nil, fmt.Errorf("production pages use incompatible transport: %w", ErrInvalid)
			}
		}

		for i, receiver := range sets[index].Receivers {
			if identities[receiver.ID] || results[index].Estimates[i].ReceiverID != receiver.ID {
				return nil, nil, fmt.Errorf("production receiver collision or order mismatch %d: %w", receiver.ID, ErrInvalid)
			}

			identities[receiver.ID] = true
		}

		offsets[index+1] = offsets[index] + len(sets[index].Receivers)
	}

	if plugin.options.Reconstruction == nil || offsets[len(chunk.Pages)] == 0 {
		return sets, results, nil
	}

	receivers = plugin.pageInputs.group(productionReceiverGroupKey(scene, chunk.Pages))
	if len(receivers) != offsets[len(chunk.Pages)] {
		receivers = make([]bake.Receiver, offsets[len(chunk.Pages)])
		for index := range sets {
			copy(receivers[offsets[index]:], sets[index].Receivers)
		}
	}

	estimates = make([]bake.Estimate, len(receivers))
	for index := range sets {
		copy(estimates[offsets[index]:], results[index].Estimates)
	}

	if err := plugin.openWorker(ctx); err != nil {
		return nil, nil, err
	}

	options := *plugin.options.Reconstruction

	graph, err := readProductionSupportReport(
		ctx,
		support,
		scene.GeometryHash(),
		receivers,
		chunk.Pages,
		plugin.options.SurfaceEdges.MaximumEdges,
	)
	if err != nil {
		return nil, nil, err
	}

	reconstructed, err := plugin.worker.ReconstructPrepared(ctx, scene, receivers, estimates, graph.Prepared, options)
	if err != nil {
		return nil, nil, err
	}

	if len(reconstructed) != len(receivers) {
		return nil, nil, ErrInvalid
	}

	isolated := make(map[uint64]bool, len(graph.IsolatedReceiverIDs))
	for _, id := range graph.IsolatedReceiverIDs {
		isolated[id] = true
	}

	for index, estimate := range estimates {
		if isolated[estimate.ReceiverID] {
			reconstructed[index] = estimate
		}
	}

	for page := range results {
		results[page].Estimates = reconstructed[offsets[page]:offsets[page+1]:offsets[page+1]]
		for i, estimate := range results[page].Estimates {
			if estimate.ReceiverID != sets[page].Receivers[i].ID {
				return nil, nil, ErrInvalid
			}
		}
	}

	return sets, results, nil
}
