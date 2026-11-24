//go:build ignore

package main

import (
	"os"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
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

		primary := assetbuild.KTX2Options{UsageFormats: map[string]texture.Format{
			assetbuild.UsageAlbedo: texture.BC1,
			assetbuild.UsageNormal: texture.BC7,
			assetbuild.UsageSSBump: texture.BC7,
		}}
		// Match the BC7 encoder's UASTC search effort for mobile coefficients.
		quality := 2
		mobile := assetbuild.KTX2Options{
			Compression: assetbuild.BlockCompressionOptions{SSBumpQuality: &quality},
			UsageFormats: map[string]texture.Format{
				assetbuild.UsageAlbedo: texture.ETC2RGB,
				assetbuild.UsageNormal: texture.ETC2RGBA,
				assetbuild.UsageSSBump: texture.ASTC4x4,
			},
		}

		return assetbuild.Config{
			Output: "generated/assets",
			Pipelines: []assetbuild.Pipeline{{
				Name: "standardMaterials", Input: inputs,
				Import:  []assetbuild.Plugin{assetbuild.Images()},
				Process: processing,
				Output:  []assetbuild.Plugin{assetbuild.KTX2Variants(primary, mobile), assetbuild.GoCatalog()},
			}},
			Profiles: []string{"dev", "release", "materialize"},
		}
	})
}
