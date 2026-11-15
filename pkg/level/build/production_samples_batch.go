//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

type productionSamplePage struct {
	Receivers, Geometry assetbuild.Artifact
}

func (plugin *productionPlugin) planSpaceSamples(
	plan *assetbuild.Plan, identity string, group *productionSpacePlan, combined chunkRecord,
) ([]assetbuild.Ref, error) {
	if plugin.options.Adaptive == nil && len(plugin.resume) == 0 && len(plugin.options.ResumeSamples) == 0 {
		if len(combined.Pages) == 0 {
			return nil, nil
		}

		return plan.Add(plugin.sampleBatchAction(identity, combined.Description.ID, combined.Pages, group.scene))
	}

	var samples []assetbuild.Ref

	for _, chunk := range group.chunks {
		for index, page := range chunk.Pages {
			sample, err := plugin.planSamples(plan, identity, chunk.Description.ID, index, page, group.scene)
			if err != nil {
				return nil, err
			}

			samples = append(samples, sample)
		}
	}

	return samples, nil
}

func (plugin *productionPlugin) sampleBatchAction(
	identity, space string, pages []pageRecord, scene assetbuild.Ref,
) assetbuild.Action {
	cache := assetbuild.CacheAutomatic
	if plugin.options.BackendIdentity == "" {
		cache = assetbuild.CacheDisabled
	}

	artifacts := make([]assetbuild.Artifact, 0, len(pages)*2)
	roster := make([]productionSamplePage, len(pages))
	outputs := make([]assetbuild.Slot, len(pages))

	for index, page := range pages {
		artifacts = append(artifacts, page.Receivers, page.Geometry)
		roster[index] = productionSamplePage{Receivers: page.Receivers, Geometry: page.Geometry}
		outputs[index] = assetbuild.Slot{Name: productionSampleSlot(index), Schema: "karty.lightmapper-sampled-page@1"}
	}

	return assetbuild.Action{
		Parallel: true, Exclusive: plugin.workerResource,
		ID:        "level/production/samples/" + identity + "/" + space,
		Processor: "lightmapper-sample", Revision: "go-webgpu-linear-batches@1",
		Inputs: []assetbuild.Ref{scene}, Artifacts: artifacts,
		Options: struct {
			Sampling bake.Options
			Worker   bake.WorkerOptions
			Backend  string
			Pages    []productionSamplePage
		}{plugin.options.Sampling, plugin.options.Worker, plugin.options.BackendIdentity, roster},
		Cache: cache, Outputs: outputs,
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return plugin.sampleBatches(ctx, pages, scene)
		},
	}
}

func productionSampleSlot(index int) string { return fmt.Sprintf("page-%d", index) }

func (plugin *productionPlugin) sampleBatches(
	ctx *assetbuild.Context, pages []pageRecord, ref assetbuild.Ref,
) (assetbuild.Result, error) {
	scene, err := plugin.readScene(ctx, ref)
	if err != nil {
		return nil, err
	}

	batches := make([]bake.BakeBatch, len(pages))

	for index, page := range pages {
		set, readErr := plugin.readProductionReceiverPage(ctx, scene, page, ctx.ReadArtifact)
		if readErr != nil {
			return nil, readErr
		}

		batches[index] = bake.BakeBatch{Receivers: set.Receivers, Options: plugin.options.Sampling}
	}

	if err = plugin.openWorker(ctx); err != nil {
		return nil, err
	}

	linear, err := plugin.worker.BakeBatches(ctx, scene, batches)
	if err != nil {
		return nil, err
	}

	if len(linear) != len(pages) {
		return nil, ErrInvalid
	}

	// BakeBatches admits the complete ordered result roster before returning.
	// Store one encoded page at a time; no output reference escapes this action
	// unless every page succeeds. Failed actions do not acquire a cache entry.
	return productionSampleBatchResult(linear, ctx.StoreArtifact)
}

func productionSampleBatchResult(
	linear []bake.Result, store func([]byte) (assetbuild.Artifact, error),
) (assetbuild.Result, error) {
	output := make(assetbuild.Result, len(linear))

	for index, page := range linear {
		encoded, err := bake.EncodeResult(page)
		if err != nil {
			return nil, err
		}

		artifact, err := store(encoded)
		if err != nil {
			return nil, err
		}

		data, err := encode(productionSample{Linear: artifact})
		if err != nil {
			return nil, err
		}

		output[productionSampleSlot(index)] = data
	}

	return output, nil
}
