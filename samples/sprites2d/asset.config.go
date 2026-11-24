//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	spritebuild "github.com/karty-game/ashura/pkg/draw2d/build"
)

func main() {
	quality := 2

	assetbuild.Run(assetbuild.Config{
		Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{
			Name: "sprites2d", Input: []string{"assets/**"},
			Import: []assetbuild.Plugin{assetbuild.Images()},
			Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
				Name: "sprites", Include: []string{"sprites/**"},
				Clamp: true, Padding: 2,
			})},
			Output: []assetbuild.Plugin{spritebuild.PortableImages(assetbuild.BlockCompressionOptions{LDRQuality: &quality})},
		}},
	})
}
