//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	uibuild "github.com/karty-game/ashura/pkg/ui/build"
)

func main() {
	assetbuild.Run(assetbuild.Config{
		Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{
			uibuild.Atlas(uibuild.AtlasOptions{Name: "ui", Include: "ui/**", MaxSize: 512, Padding: 2}),
		},
	})
}
