# Ashura

![Ashura — Go-first WebGPU rendering](branding/banner-readme.png)

A Go-first WebGPU engine built around one mantra:

> Do as much work as possible ahead of time so you have to do as little as possible at runtime.

## Features

- **Browser and native Linux rendering** with a shared Go API and WebGPU backend.
- **3D scenes** with meshes, instancing, materials, lights, shadows and prepared reflection maps.
- **2D sprites** with prepared atlases, transforms, clipping, tint and opacity.
- **UI Library** with layout, controls, text editing, motion and virtual lists. Keyboard operation and semantic metadata are supported; platform screen-reader bridges are not implemented.
- **Offline asset preparation** advanced and customizable asset pipeline to prepare models, textures, atlases, lighting and reflection maps.
- **Go First** No CGO, compile your Game with Go or Tinygo.

## Getting Started

```go
package main

import (
	"github.com/karty-game/karty-webgpu/pkg/camera"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/engine/features"
	"github.com/karty-game/karty-webgpu/pkg/light"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/mesh"
)

func main() {
	// Initialize Engine & World
	app := engine.NewApp(engine.AppConfig{Title: "Cube", CanvasID: "game"})
	defer app.Close()
	if err := app.Register(features.Standard()); err != nil {
		panic(err)
	}

	world := engine.NewWorld()

	// Create Orbit Camera
	projection, orbit := camera.Orbit()
	orbit.Distance = 3
	world.Add(engine.DefaultTransform(), projection, orbit)

	// Create Light
	world.Add(light.DefaultHemisphere("sky"))

	// Create Cube
	pos := engine.DefaultTransform()
	pos.Rotation = engine.Vector3{X: .4, Y: .6}
	world.Add(mesh.NewBox("cube", mesh.BoxOptions{}), pos, material.DefaultStandard())

	// Render World
	if err := app.Run(world); err != nil {
		panic(err)
	}
}
```
