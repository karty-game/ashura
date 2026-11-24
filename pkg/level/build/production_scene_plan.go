//go:build !js

package levelbuild

import "github.com/karty-game/ashura/pkg/assetbuild"

type productionSceneRefs struct{ geometry, lighting assetbuild.Ref }

func (plugin *productionPlugin) planScene(
	plan *assetbuild.Plan,
	identity, key string,
	record partitioned,
	chunk chunkRecord,
) (productionSceneRefs, error) {
	geometry, err := plan.Add(
		assetbuild.Action{
			Parallel:  true,
			ID:        "level/production/geometry/" + identity + "/" + key,
			Processor: "lightmapper-geometry",
			Revision:  "prepared-acceleration@4",
			Artifacts: []assetbuild.Artifact{chunk.Occluders},
			Options: struct {
				Source, Space string
				MetresPerUnit float32
			}{record.Source, chunk.Description.Space, plugin.options.MetresPerUnit},
			Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.lightmapper-scene-index@2"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return plugin.prepareGeometry(ctx, record, chunk)
			},
		},
	)
	if err != nil {
		return productionSceneRefs{}, err
	}

	lighting := plugin.lighting(record, chunk.Description.Space)

	refs, err := plan.Add(
		assetbuild.Action{
			Parallel:  true,
			ID:        "level/production/scene/" + identity + "/" + key,
			Processor: "lightmapper-lighting",
			Revision:  "prepared-light-hierarchy@3",
			Inputs:    geometry,
			Options:   lighting,
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.lightmapper-scene-index@2"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return plugin.prepareLighting(ctx, geometry[0], lighting)
			},
		},
	)
	if err != nil {
		return productionSceneRefs{}, err
	}

	return productionSceneRefs{geometry: geometry[0], lighting: refs[0]}, nil
}
