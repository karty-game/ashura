//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	reflectionbuild "github.com/karty-game/ashura/pkg/reflection/build"
	"github.com/karty-game/ashura/pkg/texture"
)

func main() {
	assetbuild.Run(assetbuild.Config{
		Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{
			Name:  "reflections",
			Input: []string{"assets/room.json", "assets/chunks/*.json"},
			Import: []assetbuild.Plugin{
				reflectionbuild.Scene(
					reflectionbuild.SceneOptions{Name: "Room", Source: "assets/room.json", Chunks: "assets/chunks/*.json"},
				),
			},
			Process: []assetbuild.Plugin{assetbuild.ReflectionProbes()},
			Output: []assetbuild.Plugin{
				assetbuild.KTX2Variants(assetbuild.KTX2Options{Format: texture.BC1}, assetbuild.KTX2Options{Format: texture.ETC2RGB}),
				assetbuild.GoCatalog(),
			},
		}},
	})
}
