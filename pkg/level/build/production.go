//go:build !js

package levelbuild

import (
	"fmt"
	"sync"

	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

// ProductionOptions configures the shared offline RGB/RNM compute lightmapper.
// Explicit preview jobs retain quality diagnostics but cannot claim final
// directional verification. Memory is a build-worker ceiling, not a frame quota.
type ProductionOptions struct {
	// UVs prepares production receiver UVs from StaticChunks in this module,
	// overlapping independent scene acceleration and UV preparation. Nil accepts
	// the existing separately prepared LightmapUVs output.
	UVs *UVOptions
	// ResumeSamples selects explicit checksummed fixed-count linear build inputs.
	ResumeSamples  []string
	Worker         bake.WorkerOptions
	Sampling       bake.Options
	Adaptive       *bake.AdaptivePolicy
	Reconstruction *bake.ReconstructionOptions
	SurfaceEdges   bake.SurfaceEdgeOptions
	Verification   *bake.VerificationPolicy
	Preview        bool
	MetresPerUnit  float32
	Environment    *bake.Environment
	Emitters       map[string][]bake.Emitter
	// BackendIdentity overrides the identity reported by the native adapter.
	BackendIdentity string
}

type productionPlugin struct {
	pageInputs                      *productionPageCache
	sceneMu                         sync.Mutex
	supportResource, workerResource *assetbuild.ExecutionResource
	options                         ProductionOptions
	worker                          *bake.Worker
	backend                         assetbuild.Ref
	resume                          []assetbuild.Ref
	sceneKey                        string
	scene                           *bake.PreparedScene
	earlyScenes                     map[string]productionSceneRefs
}

type productionScene struct{ Data assetbuild.Artifact }
type productionSample struct {
	Linear  assetbuild.Artifact
	Resumed bool
}

// ProductionBake uses one transport estimator for physical RGB and calibrated
// RNM. Supply UVs after StaticChunks, or use separate production LightmapUVs.
// Publish selects representation and native device family without another trace.
func ProductionBake(options ProductionOptions) assetbuild.Plugin {
	if options.MetresPerUnit == 0 {
		options.MetresPerUnit = 1
	}

	if options.Sampling.Outputs == 0 {
		options.Sampling.Outputs = bake.OutputRGB | bake.OutputRNM
	}

	return &productionPlugin{options: options}
}
func (*productionPlugin) Name() string { return "level-production-lightmapper" }
func (plugin *productionPlugin) Plan(plan *assetbuild.Plan) error {
	plugin.earlyScenes = make(map[string]productionSceneRefs)
	plugin.pageInputs = &productionPageCache{}
	plugin.supportResource = assetbuild.NewExecutionResource()
	plugin.workerResource = assetbuild.NewExecutionResource()
	plugin.backend = assetbuild.Ref{}

	if err := plugin.planResumeInputs(plan); err != nil {
		return err
	}

	if plugin.options.Worker.MemoryBytes == 0 {
		return fmt.Errorf("production lightmapper requires an explicit build memory ceiling: %w", ErrInvalid)
	}

	if !plugin.options.Preview && plugin.options.Sampling.Outputs&bake.OutputRNM != 0 && plugin.options.Verification == nil {
		return fmt.Errorf("final RNM requires directional verification: %w", ErrInvalid)
	}

	plan.Cleanup(func() error {
		plugin.pageInputs = nil
		if plugin.worker == nil {
			return nil
		}

		err := plugin.worker.Close()
		plugin.worker = nil
		plugin.scene = nil

		return err
	})

	if plugin.options.UVs != nil {
		if err := plugin.planIntegratedUVs(plan, *plugin.options.UVs); err != nil {
			return err
		}
	}

	return (stage{name: "production-bake", schema: uvSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(assetbuild.Expansion{ID: "level/production/discover/" + resource.ID, Inputs: []assetbuild.Ref{resource.Ref}, Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			return plugin.discover(ctx, child, resource)
		}})
	}}).Plan(plan)
}

func (plugin *productionPlugin) discover(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource) error {
	var record partitioned
	if err := readRecord(ctx, resource.Ref, &record); err != nil {
		return err
	}

	chunks, err := plugin.planProductionSpaces(plan, resource.ID, record)
	if err != nil {
		return err
	}

	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "level/production/index/" + resource.ID,
			Processor: "lightmapper-index",
			Revision:  "complete-family@1",
			Inputs:    append([]assetbuild.Ref{resource.Ref}, chunks...),
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return collectProductionChunks(ctx, resource.Ref, chunks)
			},
		},
	)
	if err != nil {
		return err
	}

	replaceResource(plan, resource.ID, refs[0])

	return nil
}

func collectProductionChunks(ctx *assetbuild.Context, source assetbuild.Ref, chunks []assetbuild.Ref) (assetbuild.Result, error) {
	var record partitioned
	if err := readRecord(ctx, source, &record); err != nil {
		return nil, err
	}

	if len(record.Chunks) != len(chunks) {
		return nil, ErrInvalid
	}

	for index, ref := range chunks {
		var pages []pageRecord
		if err := readRecord(ctx, ref, &pages); err != nil {
			return nil, err
		}

		if len(pages) != len(record.Chunks[index].Pages) {
			return nil, ErrInvalid
		}

		record.Chunks[index].Pages = pages
	}

	return result(record)
}
