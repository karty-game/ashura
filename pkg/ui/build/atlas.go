//go:build !js

package build

import "github.com/karty-game/ashura/pkg/assetbuild"

// AtlasOptions selects the UI artwork group within assets/. Include matches
// extensionless names relative to that directory, for example "ui/**".
type AtlasOptions struct {
	Name    string
	Include string
	MaxSize int
	Padding int
}

// Atlas imports images under assets/, packs the chosen clamped UI group, and
// publishes a UI catalog. Artwork outside the group keeps standalone pages.
func Atlas(options AtlasOptions) assetbuild.Pipeline {
	return assetbuild.Pipeline{
		Name:   options.Name,
		Input:  []string{"assets/**"},
		Import: []assetbuild.Plugin{assetbuild.Images()},
		Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
			Name: options.Name, Include: []string{options.Include}, MaxSize: options.MaxSize, Padding: options.Padding, Clamp: true,
		})},
		Output: []assetbuild.Plugin{Images()},
	}
}
