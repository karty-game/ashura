//go:build ignore

package main

import (
	"os"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func main() {
	assetbuild.Main(func(environment assetbuild.Environment) assetbuild.Config {
		inputs := []string{"assets/**"}
		var processing []assetbuild.Plugin
		if environment.Profile == "materialize" {
			inputs = []string{"assets/*-albedo.png", "assets/*-ao.png"}
			processing = append(processing, assetbuild.Materialize(assetbuild.MaterializeOptions{
				Maps:            []string{"normal"},
				BackendIdentity: os.Getenv("MATERIALIZE_BACKEND_ID"), Seamless: true,
			}))
		}

		processing = append(processing,
			assetbuild.SSBump(assetbuild.SSBumpOptions{Include: []string{"*-normal"}, KeepNormals: true}),
			assetbuild.Mipmaps(),
		)

		return assetbuild.Config{
			Output: "generated/assets",
			Pipelines: []assetbuild.Pipeline{{
				Name: "standardMaterials", Input: inputs,
				Import:  []assetbuild.Plugin{assetbuild.Images()},
				Process: processing,
				Output: []assetbuild.Plugin{assetbuild.KTX2(assetbuild.KTX2Options{
					UsageFormats: map[string]texture.Format{
						assetbuild.UsageAlbedo: texture.BC1,
						assetbuild.UsageNormal: texture.BC7,
						assetbuild.UsageSSBump: texture.BC7,
					},
				}), assetbuild.GoCatalog()},
			}},
			Profiles: []string{"dev", "release", "materialize"},
		}
	})
}
