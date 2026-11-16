//go:build !js

package main

import "strings"

// Repository sample titles, descriptions and order are presentation metadata. Other browser
// projects and newly discovered samples retain their ordinary generated names.
func repositorySamplePresentation(importPath string) (string, string, int) {
	const prefix = "github.com/karty-game/karty-webgpu/samples/"

	directory, local := strings.CutPrefix(importPath, prefix)

	samples := [...]struct{ directory, title, description string }{
		{"basic-scene", "Basic scene", "The simplest scene example, built with TinyGo. Under 300 KB of gzipped WASM."},
		{"standard-materials", "Materials", "Explore different types of materials and their surface properties."},
		{"ui", "UI Components", "Explore the Ashura UI library and its set of interactive components."},
		{"sprites2d", "Sprites2D", "Build an animated 2D scene with sprites, texture atlases, layers and clipping."},
		{"billboards", "Billboards", "Integrate different types of billboards into a 3D scene."},
		{"impostors", "Impostors", "Replace distant objects with low-cost representations of their appearance."},
		{"instancing", "Instancing", "Use GPU instancing to render thousands of copies of an object."},
		{"reflections", "Reflections", "Use probes and reflection textures to simulate metallic and mirror-like surfaces."},
		{"compute-texture", "Compute texture", "Use a compute shader to generate an animated texture at runtime."},
		{"sponza", "Sponza", "Load the well-known Sponza GLB scene with baked lightmaps."},
	}
	if local {
		for order, sample := range samples {
			if directory == sample.directory {
				return sample.title, sample.description, order
			}
		}
	}

	return "", "", len(samples)
}
