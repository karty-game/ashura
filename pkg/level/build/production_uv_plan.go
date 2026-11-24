//go:build !js

package levelbuild

import "github.com/karty-game/ashura/pkg/assetbuild"

func (plugin *productionPlugin) planIntegratedUVs(plan *assetbuild.Plan, options UVOptions) error {
	options.Production = true
	preparer := newUVPlugin(options)
	preparer.receiverCache = plugin.pageInputs

	if err := preparer.configure(plan); err != nil {
		return err
	}

	return (stage{name: "production-uv", schema: chunksSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(assetbuild.Expansion{
			ID: "level/production/uv-discover/" + resource.ID, Inputs: []assetbuild.Ref{resource.Ref},
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				var record partitioned
				if err := readRecord(ctx, resource.Ref, &record); err != nil {
					return err
				}

				pageRefs := make([]assetbuild.Ref, len(record.Chunks))
				for index, chunk := range record.Chunks {
					ref, err := plugin.planIntegratedUVJobs(child, resource.ID, record, chunk, preparer)
					if err != nil {
						return err
					}

					pageRefs[index] = ref
				}

				return preparer.planUVIndex(child, resource, pageRefs)
			},
		})
	}}).Plan(plan)
}

func (plugin *productionPlugin) planIntegratedUVJobs(plan *assetbuild.Plan, identity string,
	record partitioned, chunk chunkRecord, preparer *uvPlugin) (assetbuild.Ref, error) {
	roster, err := addReceiverJobs(plan, identity, chunk)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	err = plan.Expand(assetbuild.Expansion{
		ID: "level/receiver-jobs/discover/" + identity + "/" + chunk.Description.ID, Inputs: roster,
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			var jobs []assetbuild.Artifact
			if err := readRecord(ctx, roster[0], &jobs); err != nil {
				return err
			}

			key := chunk.Description.Space + "/" + chunk.Occluders.SHA256
			if _, exists := plugin.earlyScenes[identity+"/"+key]; !exists {
				scene, err := plugin.planScene(child, identity, key, record, chunk)
				if err != nil {
					return err
				}

				plugin.earlyScenes[identity+"/"+key] = scene
			}

			return preparer.discoverUVJobs(child, identity, chunk, jobs)
		},
	})

	return assetbuild.Ref{Action: "level/prepare-index/" + identity + "/" + chunk.Description.ID, Slot: slot}, err
}
