// Package features selects optional engine implementations explicitly.
// Register only what an application uses before starting its App.
package features

import "github.com/karty-game/karty-webgpu/pkg/engine"

// Standard enables standard materials, including their shared texture bindings.
func Standard() engine.Feature { return standard() }

// Source enables Source-style directional lightmaps and relief materials.
func Source() engine.Feature { return source() }

// Impostors enables prepared impostor materials and their renderer.
func Impostors() engine.Feature { return impostors() }

// PointShadows enables point-shadow passes and their optional diagnostic view.
func PointShadows() engine.Feature { return pointShadows() }

// Sprites enables the retained Screen2D renderer.
func Sprites() engine.Feature { return sprites() }

// Compute enables declared compute resources and dispatches.
func Compute() engine.Feature { return compute() }
