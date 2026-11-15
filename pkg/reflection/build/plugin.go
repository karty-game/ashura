//go:build !js

// Package reflectionbuild prepares local primitive-scene panoramas and typed scene declarations offline.
package reflectionbuild

import (
	"fmt"
	"go/token"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// SceneOptions identifies one authored primitive scene and its chunk records.
// Name is a Go identifier used by the generated <Name>Scene declaration and texture catalog.
type SceneOptions struct{ Name, Source, Chunks string }
type plugin struct{ options SceneOptions }

// Scene imports a bounded analytic diffuse panorama and matching typed scene.
// Put ReflectionProbes after it, then KTX2 and GoCatalog. No runtime scene capture occurs.
func Scene(options SceneOptions) assetbuild.Plugin { return plugin{options: options} }

func (plugin) Name() string { return "reflection-panorama" }

func (p plugin) Plan(plan *assetbuild.Plan) error {
	if !token.IsIdentifier(p.options.Name) || !token.IsExported(p.options.Name) || p.options.Source == "" || p.options.Chunks == "" {
		return fmt.Errorf("reflection scene requires an exported Name, Source and Chunks: %w", errSource)
	}

	name := strings.ToLower(p.options.Name)

	authored, neighborhood, err := loadSource(plan, p.options)
	if err != nil {
		return err
	}

	if err := planRuntimeScene(plan, authored, p.options.Name); err != nil {
		return err
	}

	refs, err := plan.Add(assetbuild.Action{
		ID: name + "-panorama", Processor: "reflection-panorama", Revision: "chunk-neighborhood-32m@3",
		Options: neighborhood, Outputs: []assetbuild.Slot{{Name: "images", Schema: assetbuild.ImageSetSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			pixels, renderErr := render(ctx, neighborhood)
			if renderErr != nil {
				return nil, renderErr
			}

			data, encodeErr := assetbuild.EncodeImageSet(assetbuild.ImageSet{Images: []assetbuild.Image{{
				Name: p.options.Name + "Reflection", Source: name + "-reflection.png", Usage: assetbuild.UsageReflection,
				Width: pixels.Bounds().Dx(), Height: pixels.Bounds().Dy(), Levels: [][]byte{pixels.Pix},
			}}})

			return assetbuild.Result{"images": data}, encodeErr
		},
	})
	if err != nil {
		return err
	}

	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: name + "-reflection", Ref: refs[0]})

	return nil
}
