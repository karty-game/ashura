package main

import (
	"errors"
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/pkg/reflection"
	"github.com/karty-game/ashura/pkg/reflection/scene"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
	"github.com/karty-game/ashura/samples/internal/sample"
	"github.com/karty-game/ashura/samples/reflections/generated/assets"
)

var errReflectionCompression = errors.New("reflections requires BC or ETC2 texture compression")

//nolint:mnd // The camera starts inside the authored reflection room.
func createScene(app *engine.App) (*engine.World, error) {
	scene := engine.NewWorld()
	projection, controls := camera.FirstPerson()

	view := scene.Add(
		engine.Transform{Position: engine.Vector3{Y: 2.2, Z: 5.2}, Rotation: engine.Vector3{X: -.08}, Scale: engine.Vector3One()},
		projection, controls,
	)
	if err := scene.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, scene, view)

	authored := assets.RoomScene()
	scene.Add(light.Hemisphere{
		Name: authored.Fill.Name, Direction: sceneVector(authored.Fill.Direction), Intensity: authored.Fill.Intensity,
		SkyColor: sceneColor(authored.Fill.SkyColor), GroundColor: sceneColor(authored.Fill.GroundColor),
	})
	scene.Add(light.Directional{
		Name: authored.Key.Name, Direction: sceneVector(authored.Key.Direction), Intensity: authored.Key.Intensity,
		Color: sceneColor(authored.Key.Color),
	})

	// The offline panorama and visible geometry share this room definition.
	// The placed probe demonstrates cubemap parallax correction within its box.
	probe := reflection.DefaultProbe("room probe", nil)
	probe.HalfExtents = sceneVector(authored.ProbeHalfExtents)
	probeEntity := scene.Add(probe, engine.Transform{Position: sceneVector(authored.ProbePosition), Scale: engine.Vector3One()})

	var reflective []*material.Material

	for _, object := range authored.Objects {
		surface := material.DefaultStandard()
		surface.Color = sceneColor(object.Surface.Color)
		surface.Roughness = object.Surface.Roughness
		surface.ReflectionStrength = object.Surface.ReflectionStrength

		entity := scene.Add(geometry(object), engine.Transform{
			Position: sceneVector(object.Position), Rotation: sceneVector(object.Rotation), Scale: engine.Vector3One(),
		}, surface)
		if surface.ReflectionStrength > 0 {
			component, _ := engine.Get[material.Material](scene, entity)
			reflective = append(reflective, component)
		}
	}

	liveProbe, _ := engine.Get[reflection.Probe](scene, probeEntity)

	var stop func()

	stop = app.OnUpdateError(func(time.Duration) error {
		stop()

		asset, err := reflectionForFormat(app.CompressedAlbedoFormat())
		if err != nil {
			return err
		}

		liveProbe.Environment, err = asset.Load(app)

		return err
	})

	if err := showReflectionControls(app, liveProbe, reflective); err != nil {
		return nil, err
	}

	return scene, nil
}

func reflectionForFormat(format texture.Format) (ktx2.Asset, error) {
	if format == texture.BC1 {
		return assets.RoomReflection(), nil
	}

	if format == texture.ETC2RGB {
		return assets.RoomReflectionMobile(), nil
	}

	return ktx2.Asset{}, errReflectionCompression
}

func geometry(object reflectionscene.Object) mesh.Mesh {
	switch object.Shape {
	case reflectionscene.Box:
		return mesh.NewBox(object.Name, mesh.BoxOptions{Width: object.Size.X, Height: object.Size.Y, Depth: object.Size.Z})
	case reflectionscene.Sphere:
		return mesh.NewSphere(object.Name, mesh.SphereOptions{Diameter: object.Size.X, Segments: reflectionscene.SphereSegments})
	case reflectionscene.Plane:
		return mesh.NewPlane(object.Name, mesh.PlaneOptions{Width: object.Size.X, Height: object.Size.Z})
	default:
		panic("unknown authored room primitive")
	}
}

func sceneVector(value reflectionscene.Vector3) engine.Vector3 {
	return engine.Vector3{X: value.X, Y: value.Y, Z: value.Z}
}

func sceneColor(value reflectionscene.Color) engine.Color {
	return engine.Color{R: value.R, G: value.G, B: value.B, A: value.A}
}
