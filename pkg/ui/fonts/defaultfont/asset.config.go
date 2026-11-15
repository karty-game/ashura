//go:build ignore

package main

import (
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	uibuild "github.com/karty-game/karty-webgpu/pkg/ui/build"
)

func main() {
	assetbuild.Main(func(assetbuild.Environment) assetbuild.Config {
		return assetbuild.Config{
			Output: "prepared",
			Pipelines: []assetbuild.Pipeline{{
				Name: "defaultFont", Output: []assetbuild.Plugin{uibuild.DefaultFont()},
			}},
		}
	})
}
