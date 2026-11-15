//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func (plugin *productionPlugin) planSamples(
	plan *assetbuild.Plan,
	identity, chunk string,
	index int,
	page pageRecord,
	scene assetbuild.Ref,
) (assetbuild.Ref, error) {
	cache := assetbuild.CacheAutomatic
	if plugin.options.BackendIdentity == "" {
		cache = assetbuild.CacheDisabled
	}

	refs, err := plan.Add(
		assetbuild.Action{
			Parallel: true, Exclusive: plugin.workerResource,
			ID:        fmt.Sprintf("level/production/sample/%s/%s/%d", identity, chunk, index),
			Processor: "lightmapper-sample",
			Revision:  "go-webgpu-linear-result@1",
			Inputs:    append([]assetbuild.Ref{scene}, plugin.resume...),
			Artifacts: []assetbuild.Artifact{page.Receivers, page.Geometry},
			Options: struct {
				Sampling bake.Options
				Adaptive *bake.AdaptivePolicy
				Worker   bake.WorkerOptions
				Backend  string
			}{plugin.options.Sampling, plugin.options.Adaptive, plugin.options.Worker, plugin.options.BackendIdentity},
			Cache:   cache,
			Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.lightmapper-sampled-page@1"}},
			Run:     func(ctx *assetbuild.Context) (assetbuild.Result, error) { return plugin.sample(ctx, page, scene) },
		},
	)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}
func (plugin *productionPlugin) sample(ctx *assetbuild.Context, page pageRecord, ref assetbuild.Ref) (assetbuild.Result, error) {
	scene, err := plugin.readScene(ctx, ref)
	if err != nil {
		return nil, err
	}

	set, err := plugin.readProductionReceiverPage(ctx, scene, page, ctx.ReadArtifact)
	if err != nil {
		return nil, err
	}

	if err = plugin.openWorker(ctx); err != nil {
		return nil, err
	}

	linear, resumed, err := plugin.resumeSample(ctx, scene, set.Receivers)
	if err != nil {
		return nil, err
	}

	if !resumed {
		if plugin.options.Adaptive != nil {
			linear, _, err = plugin.worker.BakeAdaptive(
				ctx,
				scene,
				set.Receivers,
				plugin.options.Sampling,
				*plugin.options.Adaptive,
				nil,
				nil,
			)
		} else {
			linear, err = plugin.worker.Bake(ctx, scene, set.Receivers, plugin.options.Sampling)
		}
	}

	if err != nil {
		return nil, err
	}

	encoded, err := bake.EncodeResult(linear)
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(encoded)
	if err != nil {
		return nil, err
	}

	return result(productionSample{Linear: artifact, Resumed: resumed})
}
