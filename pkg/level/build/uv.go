//go:build !js

package levelbuild

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	preparation "github.com/karty-game/karty-webgpu/internal/lightmapbuild"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// LightmapUVs prepares density-controlled RNM pages and full-space transport per chunk.
func LightmapUVs(options UVOptions) assetbuild.Plugin {
	return newUVPlugin(options)
}

func newUVPlugin(options UVOptions) *uvPlugin {
	if options.TexelsPerUnit == 0 {
		options.TexelsPerUnit = 16
	}

	if options.MaxPageSize == 0 {
		options.MaxPageSize = 1024
	}

	if options.Padding == 0 {
		options.Padding = 4
	}

	if options.TransportSamples == 0 {
		options.TransportSamples = 64
	}

	preparers := map[string]*preparation.Preparer{}

	return &uvPlugin{options: options, preparers: preparers}
}

type uvPlugin struct {
	receiverCache *productionPageCache
	options       UVOptions
	preparers     map[string]*preparation.Preparer
	productionMu  sync.Mutex
	production    *preparation.UVPreparer
}

func (*uvPlugin) Name() string { return "level-uv" }
func (plugin *uvPlugin) Plan(plan *assetbuild.Plan) error {
	if err := plugin.configure(plan); err != nil {
		return err
	}

	return (stage{name: "uv", schema: chunksSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(assetbuild.Expansion{ID: "level/uv/" + resource.ID, Inputs: []assetbuild.Ref{resource.Ref}, Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			return plugin.discoverUVPages(ctx, child, resource)
		}})
	}}).Plan(plan)
}

func (plugin *uvPlugin) configure(plan *assetbuild.Plan) error {
	options := plugin.options
	if options.MaxPageSize > 1024 || options.MaxPageSize < 16 || options.MaxPageSize&(options.MaxPageSize-1) != 0 {
		return ErrInvalid
	}

	plan.Cleanup(func() error {
		var failure error

		failure = errors.Join(failure, plugin.closeProductionPreparer())

		for key, preparer := range plugin.preparers {
			if err := preparer.Close(); err != nil {
				failure = errors.Join(failure, err)
			}

			delete(plugin.preparers, key)
		}

		return failure
	})

	return nil
}

func (plugin *uvPlugin) prepare(ctx *assetbuild.Context, chunk chunkRecord) (assetbuild.Result, error) {
	if plugin.options.Production {
		return plugin.prepareProductionUV(ctx, chunk)
	}

	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	preparer, err := plugin.sharedPreparer(ctx, bounded, chunk)
	if err != nil {
		return nil, err
	}

	data, err := ctx.ReadArtifact(chunk.Geometry)
	if err != nil {
		return nil, err
	}

	receivers, err := modeldata.Decode(data)
	if err != nil {
		return nil, err
	}

	options := plugin.options

	pages, err := preparer.PreparePages(
		bounded,
		receivers,
		preparation.PageOptions{
			RNM:                      true,
			PrecomputeTransport:      !options.PreserveGeometricNormals,
			PreserveGeometricNormals: options.PreserveGeometricNormals,
			TransportSamples:         options.TransportSamples,
			Padding:                  options.Padding,
			TexelsPerUnit:            options.TexelsPerUnit,
			MaxPageSize:              options.MaxPageSize,
		},
	)
	if err != nil {
		return nil, err
	}

	records := make([]pageRecord, 0, len(pages))
	for _, page := range pages {
		record, err := plugin.storePreparedPage(ctx, page, chunk.SSBump)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	encoded, err := json.Marshal(records)

	return assetbuild.Result{slot: encoded}, err
}

func (plugin *uvPlugin) storePreparedPage(
	ctx *assetbuild.Context,
	page preparation.Result,
	bumps map[string]assetbuild.Artifact,
) (pageRecord, error) {
	if page.Width < 16 || page.Height < 16 || page.Width > 1024 || page.Height > 1024 {
		return pageRecord{}, ErrInvalid
	}

	model, err := ctx.StoreArtifact(page.Model)
	if err != nil {
		return pageRecord{}, err
	}

	inputs, err := ctx.StoreArtifact(page.Lightmap)
	if err != nil {
		return pageRecord{}, err
	}

	var geometricNormals assetbuild.Artifact

	if plugin.options.PreserveGeometricNormals {
		geometricNormals, err = ctx.StoreArtifact(page.GeometricNormals)
		if err != nil {
			return pageRecord{}, err
		}
	}

	return pageRecord{Geometry: model, BakeInputs: inputs, GeometricNormals: geometricNormals,
		Width: uint32(page.Width), Height: uint32(page.Height), SSBump: bumps}, nil
}

func (plugin *uvPlugin) discoverUVPages(ctx *assetbuild.Context, child *assetbuild.Plan, resource assetbuild.Resource) error {
	var record partitioned
	if err := readRecord(ctx, resource.Ref, &record); err != nil {
		return err
	}

	pageRefs := make([]assetbuild.Ref, len(record.Chunks))
	for index, chunk := range record.Chunks {
		ref, err := plugin.planUVJobs(ctx, child, resource.ID, chunk)
		if err != nil {
			return err
		}

		pageRefs[index] = ref
	}

	return plugin.planUVIndex(child, resource, pageRefs)
}

func (plugin *uvPlugin) planUVIndex(child *assetbuild.Plan, resource assetbuild.Resource, pageRefs []assetbuild.Ref) error {
	return child.Expand(assetbuild.Expansion{
		ID: "level/uv/collect/" + resource.ID, Inputs: []assetbuild.Ref{resource.Ref},
		Run: func(_ *assetbuild.Context, child *assetbuild.Plan) error {
			refs, err := child.Add(
				assetbuild.Action{
					ID:        "level/uv-index/" + resource.ID,
					Processor: "level-uv-index",
					Revision:  "page-roster@1-wasi1",
					Inputs:    append([]assetbuild.Ref{resource.Ref}, pageRefs...),
					Outputs:   []assetbuild.Slot{{Name: slot, Schema: uvSchema}},
					Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
						var current partitioned
						if err := readRecord(ctx, resource.Ref, &current); err != nil {
							return nil, err
						}

						current.Density = plugin.options.TexelsPerUnit
						for index, ref := range pageRefs {
							if err := readRecord(ctx, ref, &current.Chunks[index].Pages); err != nil {
								return nil, err
							}
						}

						return result(current)
					},
				},
			)
			if err != nil {
				return err
			}

			replaceResource(child, resource.ID, refs[0])

			return nil
		},
	})
}

func (plugin *uvPlugin) sharedPreparer(
	ctx *assetbuild.Context,
	bounded context.Context,
	chunk chunkRecord,
) (*preparation.Preparer, error) {
	key := chunk.Occluders.SHA256

	preparer := plugin.preparers[key]
	if preparer == nil {
		data, err := ctx.ReadArtifact(chunk.Occluders)
		if err != nil {
			return nil, err
		}

		scene, err := modeldata.Decode(data)
		if err != nil {
			return nil, err
		}

		preparer, err = preparation.NewPreparer(bounded, scene)
		if err != nil {
			return nil, err
		}

		plugin.preparers[key] = preparer
	}

	return preparer, nil
}
