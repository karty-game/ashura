package main

import (
	_ "embed"
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/compute"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/pkg/shader"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/samples/internal/sample"
)

//go:embed wgsl/pattern.wgsl
var patternWGSL string

func createScene(app *engine.App) (*engine.World, error) {
	image, err := createPattern(app)
	if err != nil {
		return nil, err
	}

	scene := engine.NewWorld()
	projection, controls := camera.Orbit()

	const pitch, yaw, targetHeight, distance = -.35, .55, .8, 8

	controls.Target = engine.Vector3{Y: targetHeight}
	controls.Distance = distance

	view := scene.Add(projection, controls, engine.Transform{
		Rotation: engine.Vector3{X: pitch, Y: yaw}, Scale: engine.Vector3One(),
	})
	if err := scene.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, scene, view)

	const fillIntensity, roughness = .25, .65

	fill := light.DefaultHemisphere("fill")
	fill.Intensity = fillIntensity
	scene.Add(fill)

	sun := light.DefaultDirectional("sun")
	sun.Direction = engine.Vector3{X: -1, Y: -1, Z: -1}
	scene.Add(sun)

	surface := material.DefaultStandard()
	surface.AlbedoTexture, surface.Roughness = image, roughness

	const cubeSize, cubeHalfHeight, cubeX = 2, 1, -1.3

	scene.Add(mesh.NewBox("compute cube", mesh.BoxOptions{Width: cubeSize, Height: cubeSize, Depth: cubeSize}),
		engine.Transform{Position: engine.Vector3{X: cubeX, Y: cubeHalfHeight}, Scale: engine.Vector3One()}, surface)

	const sphereX, sphereSize, sphereHeight = 1.4, 1.8, .9

	scene.Add(mesh.NewSphere("compute sphere", mesh.SphereOptions{Diameter: sphereSize}),
		engine.Transform{Position: engine.Vector3{X: sphereX, Y: sphereHeight}, Scale: engine.Vector3One()}, surface)

	const groundSize, groundTiles = 12, 4

	surface.UVScale = engine.Vector2{X: groundTiles, Y: groundTiles}
	scene.Add(mesh.NewPlane("compute ground", mesh.PlaneOptions{Width: groundSize, Height: groundSize}),
		engine.DefaultTransform(), surface)

	return scene, nil
}

func createPattern(app *engine.App) (*texture.Texture, error) {
	const textureSize, uniformSize = 256, 16

	var frameBytes [uniformSize]byte

	params, err := shader.NewUniformBytes(app, frameBytes[:])
	if err != nil {
		return nil, err
	}

	image, err := compute.NewTexture(app, textureSize, textureSize)
	if err != nil {
		return nil, err
	}

	kernel, err := compute.NewPipeline(app, compute.PipelineDesc{
		Program: shader.WGSL("wgsl/pattern.wgsl", patternWGSL), EntryPoint: "generate_pattern",
		Layout: shader.Layout{shader.UniformLayout(0, uniformSize), shader.StorageTextureLayout(1)},
	})
	if err != nil {
		return nil, err
	}

	bindings, err := kernel.BindGroup(shader.BindBuffer(0, params), shader.BindTexture(1, image))
	if err != nil {
		return nil, err
	}

	animation := &patternAnimation{}
	if err := animation.attach(app); err != nil {
		return nil, err
	}

	initialized := false

	app.OnCompute(func(frame *engine.Frame) error {
		if animation.paused && initialized {
			return nil
		}

		binary.LittleEndian.PutUint32(frameBytes[:], math.Float32bits(float32(animation.elapsed)))

		if err := params.Write(0, frameBytes[:]); err != nil {
			return err
		}

		const workgroupSize = 8

		if err := frame.Dispatch(kernel, bindings, textureSize/workgroupSize, textureSize/workgroupSize, 1); err != nil {
			return err
		}

		initialized = true

		return nil
	})

	return image, nil
}
