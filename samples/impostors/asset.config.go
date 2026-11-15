//go:build ignore

package main

import (
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	impostorbuild "github.com/karty-game/karty-webgpu/pkg/impostor/build"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

func main() {
	assetbuild.Main(func(environment assetbuild.Environment) assetbuild.Config {
		options := impostorbuild.Options{
			Mode: impostorbuild.Octahedral, Resolution: 256, FrameGrid: 7,
			Padding: 8, MipLevels: 3,
		}
		switch environment.Profile {
		case "detailed":
			options.Resolution, options.Padding, options.MipLevels = 256, 16, 5
		case "layered":
			options = impostorbuild.Options{Mode: impostorbuild.Layered, Resolution: 128, MaxIntervals: 8, Padding: 4, MipLevels: 1}
		case "angular":
			options = impostorbuild.Options{
				Mode: impostorbuild.Angular, Resolution: 128, Azimuths: 8, Elevations: 3,
				Padding: 4, MipLevels: 3,
			}
		case "octahedral":
			options = impostorbuild.Options{
				Mode: impostorbuild.Octahedral, Resolution: 80, FrameGrid: 8,
				Padding: 4, MipLevels: 3,
			}
		}

		var processing []assetbuild.Plugin
		if options.Mode == impostorbuild.Layered {
			options.Include = []string{"assets/boulder.glb"}
			processing = append(processing, impostorbuild.Cards(options), impostorbuild.Cards(impostorbuild.Options{
				Mode: impostorbuild.Octahedral, Resolution: 64, FrameGrid: 7, Padding: 8, MipLevels: 4,
				Include: []string{"assets/stool.glb", "assets/vase.glb"},
			}))
		} else {
			processing = append(processing, impostorbuild.Cards(options))
		}

		return assetbuild.Config{
			Output: "generated",
			Pipelines: []assetbuild.Pipeline{{
				Name: "impostors", Input: []string{"assets/boulder.glb", "assets/stool.glb", "assets/vase.glb"},
				Import:  []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{MaxTextureDimension: 256})},
				Process: processing,
			}},
			Profiles: []string{"dev", "release", "detailed", "layered", "angular", "octahedral"},
		}
	})
}
