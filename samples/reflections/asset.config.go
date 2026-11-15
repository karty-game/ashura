//go:build ignore

package main

import (
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	reflectionbuild "github.com/karty-game/karty-webgpu/pkg/reflection/build"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func main() {
	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		return assetbuild.Config{
			Output: "generated/assets",
			Pipelines: []assetbuild.Pipeline{
				{
					Name:  "reflections",
					Input: []string{"assets/room.json", "assets/chunks/*.json"},
					Import: []assetbuild.Plugin{
						reflectionbuild.Scene(
							reflectionbuild.SceneOptions{Name: "Room", Source: "assets/room.json", Chunks: "assets/chunks/*.json"},
						),
					},
					Process: []assetbuild.Plugin{assetbuild.ReflectionProbes()},
					Output: []assetbuild.Plugin{
						assetbuild.KTX2(assetbuild.KTX2Options{Format: texture.BC1}), assetbuild.GoCatalog(),
					},
				},
			},
		}
	})
}
