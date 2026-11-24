//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
	"github.com/karty-game/ashura/samples/luminous/internal/bake"
)

func main() {
	assetbuild.Run(assetbuild.Config{
		Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{
			{Name: "floor", Output: []assetbuild.Plugin{bake.Floor{}}},
			{
				Name: "props", Input: []string{"assets/*.glb"},
				Import: []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{})},
			},
			{
				Name: "surfaces", Input: []string{"assets/*.jpg", "assets/castle-albedo.png"},
				Import: []assetbuild.Plugin{assetbuild.Images()},
				Process: []assetbuild.Plugin{
					assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 512}),
					assetbuild.SSBump(assetbuild.SSBumpOptions{Include: []string{"cobblestone-normal"}, Rays: 24, Distance: 12}),
					assetbuild.Mipmaps(),
				},
				Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
			},
		},
	})
}
