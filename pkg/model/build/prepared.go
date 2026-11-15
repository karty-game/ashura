//go:build !js

package modelbuild

import (
	compiler "github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// PreparedGLB exposes prepared model slots to downstream level/impostor processors
// without generating embedded catalogs or publishing the original static models.
func PreparedGLB() assetbuild.Plugin { return glbPlugin{preparedOnly: true} }

func addPreparedModel(plan *assetbuild.Plan, source string) error {
	refs, err := plan.Add(
		assetbuild.Action{
			Parallel:  true,
			ID:        "glb/" + source,
			Processor: glbProcessor,
			Revision:  "prepared-cutout@2",
			Options:   source,
			Outputs:   []assetbuild.Slot{{Name: PreparedSlot, Schema: PreparedSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := ctx.ReadFile(source)
				if err != nil {
					return nil, err
				}

				prepared, err := compiler.Compile(data, source)
				if err != nil {
					return nil, err
				}

				return assetbuild.Result{PreparedSlot: prepared.Data}, nil
			},
		},
	)
	if err != nil {
		return err
	}

	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: source, Ref: refs[0]})

	return nil
}
