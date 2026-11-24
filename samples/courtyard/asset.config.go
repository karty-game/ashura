//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	collisionbuild "github.com/karty-game/ashura/pkg/collision/build"
	"github.com/karty-game/ashura/samples/courtyard/layout"
	"github.com/karty-game/ashura/samples/internal/sampleassets"
)

func main() {
	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		return assetbuild.Config{
			Output: "generated",
			Pipelines: []assetbuild.Pipeline{
				{
					Name: "courtyard", Directory: "assets",
					Output: []assetbuild.Plugin{
						collisionbuild.Boxes("Stone", append(layout.Stone(), layout.Floor())...),
						collisionbuild.Boxes("FirstGate", layout.FirstGate()),
						collisionbuild.Boxes("TimedGate", layout.TimedGate()),
						collisionbuild.Boxes("Switch", layout.Switch()),
					},
				},
				{
					Name: "surfaces", Directory: "assets",
					Input:  []string{"assets/fps-kit/*.png"},
					Import: []assetbuild.Plugin{assetbuild.Images()},
					Process: []assetbuild.Plugin{
						assetbuild.Resize(assetbuild.ResizeOptions{
							// Bricks and concrete retain the authored 256 px detail.
							Include: []string{"*-panel", "floor-*", "crate-*", "trim-*", "decal-*"}, MaxDimension: 128,
						}),
						assetbuild.Mipmaps(),
					},
					Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
				},
				sampleassets.Credits("assets/fps-kit/LICENSE.md", "fps-kit-license.md"),
			},
		}
	})
}
