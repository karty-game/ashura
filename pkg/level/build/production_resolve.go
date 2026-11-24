//go:build !js

package levelbuild

import (
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
	"github.com/karty-game/ashura/pkg/texture"
)

func (plugin *productionPlugin) planChunkResolve(
	plan *assetbuild.Plan,
	identity string,
	chunk chunkRecord,
	scene assetbuild.Ref,
	samples []assetbuild.Ref,
	support assetbuild.Ref,
) (assetbuild.Ref, error) {
	refs, err := plan.Add(plugin.chunkResolveAction(identity, chunk, scene, samples, support))
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func (plugin *productionPlugin) chunkResolveAction(
	identity string,
	chunk chunkRecord,
	scene assetbuild.Ref,
	samples []assetbuild.Ref,
	support assetbuild.Ref,
) assetbuild.Action {
	artifacts := make([]assetbuild.Artifact, 0, 2*len(chunk.Pages))
	for _, page := range chunk.Pages {
		artifacts = append(artifacts, page.Receivers, page.Geometry)
	}

	inputs, cache := plugin.gpuCacheInputs(append([]assetbuild.Ref{scene}, samples...))
	if support.Action != "" {
		inputs = append([]assetbuild.Ref{support}, inputs...)
	}

	return assetbuild.Action{
		ID:        "level/production/resolve/" + identity + "/" + chunk.Description.ID,
		Processor: "lightmapper-resolve",
		Revision:  "go-webgpu-space-dense-support@1",
		Inputs:    inputs,
		Artifacts: artifacts,
		Options: struct {
			Production ProductionOptions
			Pages      []pageRecord
		}{plugin.options, chunk.Pages},
		Cache:   cache,
		Outputs: []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			prepared, err := plugin.readScene(ctx, scene)
			if err != nil {
				return nil, err
			}

			defer plugin.releaseProductionPages(prepared, chunk.Pages)

			sets, linear, err := plugin.reconstructChunk(ctx, chunk, prepared, samples, support)
			if err != nil {
				return nil, err
			}

			qualities, err := plugin.verifyProductionPages(ctx, prepared, sets, linear)
			if err != nil {
				return nil, err
			}

			output := make([]pageRecord, len(chunk.Pages))
			for i, page := range chunk.Pages {
				page.Quality = qualities[i]

				output[i], err = plugin.finishProductionPage(ctx, page, sets[i], linear[i])
				if err != nil {
					return nil, err
				}
			}

			return result(output)
		},
	}
}

func (plugin *productionPlugin) finishProductionPage(
	ctx *assetbuild.Context,
	page pageRecord,
	set bake.ReceiverSet,
	linear bake.Result,
) (pageRecord, error) {
	var err error

	page.Preview = plugin.options.Preview

	atlas, err := bake.EncodeAtlas(linear, set.Receivers, set.Width, set.Height, set.Gutters)
	if err != nil {
		return pageRecord{}, err
	}

	store := func(image texture.Data) (assetbuild.Artifact, error) {
		data, err := assetbuild.EncodeTexture(
			assetbuild.Image{
				Name:   "Lighting",
				Source: "lighting.ktx2",
				Usage:  assetbuild.UsageLightmap,
				Width:  image.Width,
				Height: image.Height,
				Levels: image.Levels,
			},
		)
		if err != nil {
			return assetbuild.Artifact{}, err
		}

		return ctx.StoreArtifact(data)
	}

	page.RGBMap, err = store(atlas.RGB)
	if err != nil {
		return pageRecord{}, err
	}

	page.Scale, page.Encoding = 1, rgbeEncoding
	page.Representation = leveldata.RGBDiffuse

	page.Maps = [3]assetbuild.Artifact{page.RGBMap}
	if linear.Options.Outputs&bake.OutputRNM != 0 {
		page.Representation = leveldata.RNMFit
		for i, image := range atlas.RNM {
			page.Maps[i], err = store(image)
			if err != nil {
				return pageRecord{}, err
			}
		}
	}

	return page, nil
}
