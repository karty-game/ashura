//go:build !js

package main

import "strings"

// Repository sample titles, descriptions and order are presentation metadata. Other browser
// projects and newly discovered samples retain their ordinary generated names.
func repositorySamplePresentation(importPath string) (string, string, int) {
	directory, local := strings.CutPrefix(importPath, repositorySamples)

	samples := [...]struct{ directory, title, description string }{
		{"basic-scene", "Basic scene", "Orbit a lit cube in the introductory TinyGo scene."},
		{"standard-materials", "Materials", "Explore different types of materials and their surface properties."},
		{"ui", "UI Components", "Explore the Ashura UI library and its set of interactive components."},
		{"sprites2d", "Sprites2D", "Build an animated 2D scene with sprites, texture atlases, layers and clipping."},
		{"particles", "Particles", "Replay GPU particle effects and raise a wall to see how they interact with scene depth."},
		{"billboards", "Billboards", "Integrate different types of billboards into a 3D scene."},
		{"decals", "Decals", "Spray paint onto the Switchyard and toggle its authored decals to compare the surfaces."},
		{"impostors", "Impostors", "Replace distant objects with low-cost representations of their appearance."},
		{"instancing", "Instancing", "Use GPU instancing to render thousands of copies of an object."},
		{"reflections", "Reflections", "Use probes and reflection textures to simulate metallic and mirror-like surfaces."},
		{"compute-texture", "Compute texture", "Use a compute shader to generate an animated texture at runtime."},
		{"luminous", "Luminous", "Press 1 for clustered lighting, 2 for all lights, and H to pause the comparison."},
		{"sponza", "Sponza", "Load the well-known Sponza GLB scene with baked lightmaps."},
		{"courtyard", "Collision courtyard", "Walk a collision course, aim at a switch and use pressure plates to reach the relics."},
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
