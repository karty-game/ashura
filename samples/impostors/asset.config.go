//go:build ignore

package main

import (
	"github.com/karty-game/ashura/pkg/assetbuild"
	impostorbuild "github.com/karty-game/ashura/pkg/impostor/build"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
	"github.com/karty-game/ashura/pkg/texture"
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

		return assetbuild.Config{
			Output: "generated",
			Pipelines: []assetbuild.Pipeline{
				{
					Name:   "impostors",
					Input:  []string{"assets/boulder.glb", "assets/stool.glb", "assets/vase.glb"},
					Import: []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{MaxTextureDimension: 256})},
					Process: append(
						impostorProcessors(options, "bc", texture.BC1),
						impostorProcessors(options, "mobile", texture.ASTC6x6)...),
				},
			},
			Profiles: []string{"dev", "release", "detailed", "layered", "angular", "octahedral"},
		}
	})
}

func impostorProcessors(options impostorbuild.Options, profile string, format texture.Format) []assetbuild.Plugin {
	var processing []assetbuild.Plugin
	options.TextureProfile = profile
	if options.Mode == impostorbuild.Layered {
		options.Include = []string{"assets/boulder.glb"}
		fallback := impostorbuild.Options{
			Mode: impostorbuild.Octahedral, Resolution: 64, FrameGrid: 7, Padding: 8, MipLevels: 4, AtlasFormat: format,
			Include: []string{"assets/stool.glb", "assets/vase.glb"}, TextureProfile: profile,
		}
		processing = append(processing, impostorbuild.Cards(options), impostorbuild.Cards(alignedCapture(fallback)))
	} else {
		options.AtlasFormat = format
		processing = append(processing, impostorbuild.Cards(alignedCapture(options)))
	}

	return processing
}

// Preserve capture detail and mip padding while aligning ASTC's six-pixel blocks.
func alignedCapture(options impostorbuild.Options) impostorbuild.Options {
	if options.AtlasFormat == texture.ASTC6x6 {
		stride := 1 << (options.MipLevels - 1)
		for (options.Resolution+2*options.Padding)%6 != 0 {
			options.Padding += stride
		}
	}
	return options
}
