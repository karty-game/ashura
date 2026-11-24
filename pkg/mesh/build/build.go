//go:build !js

// Package meshbuild adds offline geometry cleanup and Meshopt ordering to asset recipes.
// Import model/build.PreparedGLB and consume the processed resources in a level
// pipeline. Level-owned adapters keep UV and lighting references synchronized.
package meshbuild

import (
	"context"
	"fmt"

	"github.com/karty-game/ashura/internal/meshopt"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

type processor struct{ cleanup bool }

// Cleanup removes unused/exact duplicate vertices and zero-area faces before UV
// generation. Material parts, normals and UV seams are never welded together.
func Cleanup() assetbuild.Plugin { return processor{cleanup: true} }

// Optimize orders triangle indices for vertex-cache reuse and remaps vertex
// storage for fetch locality. Place after optional UV generation and before
// baking or publication. Resource owners adapt UV streams and derived references.
func Optimize() assetbuild.Plugin { return processor{} }

func (plugin processor) Name() string {
	if plugin.cleanup {
		return "mesh-cleanup"
	}

	return "mesh-optimize"
}

func (plugin processor) Plan(plan *assetbuild.Plan) error {
	return plan.Expand(
		assetbuild.Expansion{
			ID:  plugin.Name() + "/discover",
			Run: func(_ *assetbuild.Context, child *assetbuild.Plan) error { return plugin.processResources(child) },
		},
	)
}

func (plugin processor) processResources(plan *assetbuild.Plan) error {
	for index, resource := range plan.Resources {
		if resource.Process != nil {
			ref, err := resource.Process(plan, resource, plugin.Name())
			if err != nil {
				return err
			}

			plan.Resources[index].Ref = ref

			continue
		}

		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return err
		}

		if schema != modeldata.PreparedSchema {
			continue
		}

		refs, err := plan.Add(plugin.modelAction(resource))
		if err != nil {
			return err
		}

		plan.Resources[index].Ref = refs[0]
	}

	return nil
}

func run(
	ctx context.Context,
	model modeldata.Document,
	transform func(context.Context, modeldata.Document) (modeldata.Document, error),
) ([]byte, error) {
	model, err := transform(ctx, model)
	if err != nil {
		return nil, err
	}

	return modeldata.Encode(model)
}

func (plugin processor) modelAction(resource assetbuild.Resource) assetbuild.Action {
	return assetbuild.Action{
		Parallel:  true,
		ID:        plugin.Name() + "/" + resource.ID,
		Processor: plugin.Name(),
		Revision:  meshopt.Identity,
		Options:   plugin.cleanup,
		Inputs:    []assetbuild.Ref{resource.Ref},
		Outputs:   []assetbuild.Slot{{Name: modeldata.PreparedSlot, Schema: modeldata.PreparedSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			data, err := ctx.Input(resource.Ref)
			if err != nil {
				return nil, err
			}

			model, err := modeldata.Decode(data)
			if err != nil {
				return nil, err
			}

			transform := meshopt.Optimize
			if plugin.cleanup {
				transform = meshopt.Cleanup
			}

			output, err := run(ctx, model, transform)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", plugin.Name(), resource.ID, err)
			}

			return assetbuild.Result{modeldata.PreparedSlot: output}, nil
		},
	}
}
