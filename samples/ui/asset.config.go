//go:build ignore

package main

import (
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	uibuild "github.com/karty-game/karty-webgpu/pkg/ui/build"
)

func main() {
	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		return assetbuild.Config{
			Output: "generated/assets",
			Pipelines: []assetbuild.Pipeline{{
				Name: "ui", Input: []string{"assets/**"},
				Import: []assetbuild.Plugin{assetbuild.Images()},
				Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
					Name:    "ui",
					Include: []string{"ui/**"},
					Clamp:   true,
					MaxSize: 512,
					Padding: 2,
				})},
				Output: []assetbuild.Plugin{uibuild.Images()},
			}},
		}
	})
}
