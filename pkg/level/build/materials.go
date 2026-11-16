//go:build !js

package levelbuild

import (
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

// MaterialOptions binds processed relief resources to original model primitives.
// SSBump maps model identity -> primitive name -> processed image resource ID.
// Missing opaque relief is derived from the model's normals as before.
type MaterialOptions struct {
	SSBump map[string]map[string]string
}

// Materials prepares the imported scene's material bindings after image
// processing. It accepts role-aware images or legacy prepared KTX2 resources.
// Intermediate texture containers stay in the build cache; this does not
// publish standalone image files. Final delivery belongs to Publish/PublishVariants.
func Materials(options MaterialOptions) assetbuild.Plugin {
	return stage{name: "materials", schema: sceneSourceSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		resources, schemas, err := levelResources(plan)
		if err != nil {
			return err
		}

		inputs := []assetbuild.Ref{resource.Ref}

		for _, candidate := range plan.Resources {
			switch schemas[candidate.ID] {
			case modelbuild.PreparedSchema, assetbuild.ImageSetSchema, assetbuild.PreparedSetSchema:
				inputs = append(inputs, candidate.Ref)
			}
		}

		refs, err := plan.Add(assetbuild.Action{
			ID: "level/materials/" + resource.ID, Processor: "level-materials", Revision: "source-relief@1",
			Options: options, Inputs: inputs, Outputs: []assetbuild.Slot{{Name: slot, Schema: importSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var source sceneSource
				if err := readRecord(ctx, resource.Ref, &source); err != nil {
					return nil, err
				}

				if err := prepareSceneMaterials(ctx, &source, options, resources, schemas); err != nil {
					return nil, err
				}

				return result(source.Scene)
			},
		})
		if err == nil {
			replaceResource(plan, resource.ID, refs[0])
		}

		return err
	}}
}

func prepareSceneMaterials(ctx *assetbuild.Context, source *sceneSource, options MaterialOptions,
	resources map[string]assetbuild.Resource, schemas map[string]string) error {
	state := levelImportState{
		options:   importOptions{SSBump: options.SSBump, SeparateMasked: source.SeparateMasked},
		resources: resources, schemas: schemas,
		models: make(map[string]assetbuild.Artifact), textures: make(map[string]assetbuild.Artifact),
		documents: make(map[string]modeldata.Document), materials: make(map[string]map[string]assetbuild.Artifact),
	}
	for index := range source.Scene.Placements {
		entry := &source.Scene.Placements[index]
		// Import already separated and flattened derived cutouts. They keep their
		// original Standard materials and remain outside opaque relief processing.
		if entry.Dynamic && schemas[entry.Model] != modelbuild.PreparedSchema {
			continue
		}

		if _, err := state.modelArtifact(ctx, entry.Model); err != nil {
			return err
		}

		overrides, err := state.overrides(ctx, entry.Model)
		if err != nil {
			return err
		}

		entry.SSBump = overrides
		if !entry.Dynamic {
			entry.SSBump, err = state.staticRelief(ctx, entry.Model, overrides)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
