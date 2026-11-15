//go:build !js

package main

import "strings"

// Repository sample titles and order are presentation metadata. Other browser
// projects and newly discovered samples retain their ordinary generated names.
func repositorySamplePresentation(importPath string) (string, int) {
	const prefix = "github.com/karty-game/karty-webgpu/samples/"

	directory, local := strings.CutPrefix(importPath, prefix)

	samples := [...]struct{ directory, title string }{
		{"basic-scene", "Basic scene"},
		{"standard-materials", "Materials"},
		{"ui", "UI Components"},
		{"sprites2d", "Sprites2D"},
		{"billboards", "Billboards"},
		{"impostors", "Impostors"},
		{"instancing", "Instancing"},
		{"reflections", "Reflections"},
		{"compute-texture", "Compute texture"},
		{"sponza", "Sponza"},
	}
	if local {
		for order, sample := range samples {
			if directory == sample.directory {
				return sample.title, order
			}
		}
	}

	return "", len(samples)
}
