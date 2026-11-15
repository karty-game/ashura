//go:build ignore

package main

import (
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	spritebuild "github.com/karty-game/karty-webgpu/pkg/draw2d/build"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func main() {
	quality := 2

	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		return assetbuild.Config{
			Output: "generated/assets",
			Pipelines: []assetbuild.Pipeline{{
				Name: "sprites2d", Input: []string{"assets/**"},
				Import: []assetbuild.Plugin{assetbuild.Images()},
				Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
					Name: "sprites", Include: []string{"sprites/**"},
					Clamp: true, MaxSize: 2048, Padding: 2,
				})},
				Output: []assetbuild.Plugin{spritebuild.Images(spritebuild.ImagesOptions{
					OpaqueFormat: texture.BC1, AlphaFormat: texture.BC7,
					Compression: assetbuild.BlockCompressionOptions{LDRQuality: &quality},
				})},
			}},
		}
	})
}
