package engine

import (
	"fmt"
	"math"
	"time"

	"github.com/karty-game/ashura/internal/diagnostics"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
)

var errMeshOwner = fmt.Errorf("prepared mesh is unloaded, closed or belongs to another App: %w", ErrOwner)

func (app *App) updateFrame(world *World, deltaTime time.Duration, input platform.InputState) (render.Frame, error) {
	input = app.filterUI(input)
	app.input = input

	width, height := app.frameSize(input.Width, input.Height)
	if app.screen2D != nil {
		app.screen2D.resize(width, height)
	}

	updateCameraControls(world, deltaTime, input)

	if err := app.runUpdates(deltaTime); err != nil {
		return render.Frame{}, err
	}

	app.metrics.Step(diagnostics.UpdatePhase)

	if err := app.runCompute(); err != nil {
		return render.Frame{}, err
	}

	app.metrics.Step(diagnostics.SchedulePhase)

	frame, err := makeFrameWithLighting(world, width, height, app.featureMask.Has(platform.ClusteredLightsFeature))
	if err != nil {
		return frame, err
	}

	frame.UnculledLights = app.unculledLights
	app.scheduleParticles(&frame, deltaTime, width, height)

	if app.screen2D != nil {
		frame.Screen2D = app.screen2D.Frame(width, height)
	}

	if err := app.composeUI(&frame, input, deltaTime, width, height); err != nil {
		return frame, err
	}

	frame.Uploads = app.collectUploads()
	if err := app.validateFrameResources(frame); err != nil {
		return frame, err
	}

	app.metrics.Loaded(len(app.textures))
	app.metrics.Step(diagnostics.ScenePhase)

	return frame, nil
}

func (app *App) validateFrameResources(frame render.Frame) error {
	if frame.Decals != nil {
		if !app.featureMask.Has(platform.DecalFeature) {
			return missingFeature(platform.DecalFeature)
		}

		if err := app.validateTextures(frame.Decals.Atlas); err != nil {
			return err
		}
	}

	for _, draw := range frame.Draws {
		if app.driver != nil {
			if err := app.requireDrawFeature(draw); err != nil {
				return err
			}
		}

		if draw.MeshOwner != nil && (draw.MeshOwner != app || !draw.MeshValid) {
			return fmt.Errorf("mesh entity %d: %w", draw.ID, errMeshOwner)
		}

		if err := app.validateTextures(
			draw.Albedo,
			draw.Normal,
			draw.AO,
			draw.Reflection,
			draw.Lightmap,
			draw.RNM[0],
			draw.RNM[1],
			draw.RNM[2],
		); err != nil {
			return err
		}
	}

	return nil
}

func makeFrame(world *World, width, height int) (render.Frame, error) {
	return makeFrameWithLighting(world, width, height, false)
}

func makeFrameWithLighting(world *World, width, height int, clustered bool) (render.Frame, error) {
	if width <= 0 || height <= 0 {
		return render.Frame{}, errInvalidViewport
	}

	defer world.pruneGeometry()

	storage := &world.frame
	storage.reset()
	storage.frame.ClusteredLighting = clustered
	storage.frame.Decals = nil

	if world.decals != nil {
		if err := world.decals.snapshot(); err != nil {
			return render.Frame{}, err
		}

		decals, err := world.decals.renderFrame()
		if err != nil {
			return render.Frame{}, err
		}

		storage.frame.Decals = decals
	}

	active, found := activeCameraEntity(world)
	if found {
		storage.updateCamera(world, active, float32(width)/float32(height))
	} else {
		storage.frame.ViewProjection = [16]float32{}
		storage.frame.CameraPosition = [3]float32{}
		storage.frame.CameraNear, storage.frame.CameraFar = 0, 0
		storage.cameraReady = false
	}

	storage.collectReflectionProbes(world)

	// Match Each's mutation-safe traversal without a capturing callback, which
	// TinyGo otherwise allocates together with its captured world/storage.
	for index := 0; index < len(world.order); {
		entity := world.order[index]
		if entity.world != nil {
			entry := world.bindings[entity]
			appendBoundLights(&storage.frame, entry)

			if draw, ok := drawForBindings(world, entity, entry); ok {
				storage.applyReflection(&draw)
				storage.append(draw, entry)
			}
		}

		if index < len(world.order) && world.order[index] == entity {
			index++
		} else {
			index = world.afterEntity(entity)
		}
	}

	frame := storage.finish()

	return frame, validateFrame(frame)
}

func (storage *frameStorage) updateCamera(world *World, entity Entity, aspect float32) {
	_, _, storage.frame.PointerLock = freeControlSettings(world, entity)
	_, orbit := orbitControlSettings(world, entity)
	_, pan := panControlSettings(world, entity)
	storage.frame.PointerLock = storage.frame.PointerLock && !orbit && !pan
	projection, _ := projectionForEntity(world, entity)

	transform := *world.Transform(entity)
	if !storage.cameraReady || storage.camera != entity || storage.cameraTransform != transform || storage.projection != projection ||
		storage.aspect != aspect {
		storage.frame.ViewProjection = cameraViewProjection(transform, projection, aspect)
		storage.frame.CameraPosition = vectorArray(transform.Position)
		storage.frame.CameraNear, storage.frame.CameraFar = clipPlanes(projection.near, projection.far)
		storage.frame.PerspectiveCamera = projectionIsPerspective(projection)
		storage.camera, storage.cameraTransform, storage.projection, storage.aspect = entity, transform, projection, aspect
		storage.cameraReady = true
	}
}

func validateFrame(frame render.Frame) error {
	if len(frame.Lights) > frameLightLimit(frame) {
		if frame.ClusteredLighting {
			return errClusterLightCapacity
		}

		return errTooManyLights
	}

	if err := render.ValidateSpots(frame.Lights, frame.ClusteredLighting); err != nil {
		return err
	}

	if err := render.ValidatePointLights(frame.Lights); err != nil {
		return err
	}

	for _, draw := range frame.Draws {
		if err := render.ValidateAlphaMask(draw); err != nil {
			return err
		}

		if err := validateRNMLightmap(draw); err != nil {
			return err
		}

		if err := validateRGBLightmap(draw); err != nil {
			return err
		}

		if draw.Albedo != nil && texturedata.Space(draw.Albedo) != texturedata.SRGB && !texturedata.IsStorage(draw.Albedo) ||
			draw.Normal != nil &&
				texturedata.Space(draw.Normal) != texturedata.Linear || draw.AO != nil && texturedata.Space(draw.AO) != texturedata.Linear {
			return errTextureEncoding
		}

		if draw.Reflection != nil {
			width, height := draw.Reflection.Size()
			if texturedata.Space(draw.Reflection) != texturedata.SRGB || width%6 != 0 || width/6 != height {
				return errInvalidReflectionTexture
			}
		}
	}

	return nil
}

func frameLightLimit(frame render.Frame) int {
	if frame.ClusteredLighting {
		return render.ClusterLightLimit
	}

	return render.MaxLights
}

func appendEntityLights(frame *render.Frame, world *World, entity Entity) {
	appendBoundLights(frame, world.bindings[entity])
}

func appendBoundLights(frame *render.Frame, entry *entityBindings) {
	for _, provider := range entry.lights {
		if len(frame.Lights) > frameLightLimit(*frame) {
			return
		}

		frame.Lights = append(frame.Lights, provider.renderLight())
	}
}

func drawForEntity(world *World, entity Entity) (render.DrawCall, bool) {
	return drawForBindings(world, entity, world.bindings[entity])
}

func drawForBindings(world *World, entity Entity, entry *entityBindings) (render.DrawCall, bool) {
	draw := render.DrawCall{ID: entity.index}
	if entry.owner != nil {
		draw.MeshOwner, draw.MeshValid = entry.owner.RuntimeMeshOwner()
		if !draw.MeshValid {
			return draw, true
		}
	}

	if entry.prepared != nil {
		draw.Prepared = entry.prepared.RuntimePreparedMesh()
		if draw.Prepared == nil && entry.owner != nil {
			draw.MeshValid = false

			return draw, true
		}
	}

	if draw.Prepared == nil {
		if entry.mesh == nil {
			return render.DrawCall{}, false
		}

		positions, indices, normals, uvs, billboard := entry.mesh.RuntimeMesh()
		geometry := world.meshGeometry(positions, normals, uvs, indices)
		draw.Vertices, draw.Indices, draw.Billboard = geometry.vertices, geometry.indices, billboard
		draw.Bounds = geometry.bounds
	}

	transform := DefaultTransform()
	if entry.transform != nil {
		transform = *entry.transform
		if transform.Scale == (Vector3{}) {
			transform.Scale = Vector3One()
		}
	}

	draw.Transform = world.drawMatrix(entity, transform, draw.Billboard, entry.billboard)
	if entry.identity != nil {
		draw.InstanceKey = entry.identity.RuntimeMeshIdentity()
		draw.Instance.Model = draw.Transform
	}

	applyBoundMaterial(&draw, entry)

	if entry.decalReceiver != nil && draw.Impostor == nil {
		draw.DecalLayers = entry.decalReceiver.Layers
	}

	return draw, true
}

func applyBoundMaterial(draw *render.DrawCall, entry *entityBindings) {
	color, roughness := Color{R: 1, G: 1, B: 1, A: 1}, defaultRoughness
	if entry.material != nil {
		color, roughness = entry.material.RuntimeMaterial()
	}

	draw.Color, draw.Roughness = colorArray(color), roughness
	if entry.alphaMask != nil {
		draw.AlphaMask, draw.AlphaCutoff = entry.alphaMask.RuntimeAlphaMask()
		if draw.AlphaMask {
			draw.Color[3] = color.A
		}
	}

	draw.UVScale = [2]float32{1, 1}
	if entry.impostor != nil {
		draw.Impostor = entry.impostor.RuntimeImpostor()
		if entry.impostorQuality != nil {
			draw.ImpostorQuality = entry.impostorQuality.RuntimeImpostorQuality()
		}
	}

	if entry.textured != nil {
		albedo, normal, scale, strength := entry.textured.RuntimeTextures()
		draw.Albedo, draw.Normal, draw.UVScale, draw.NormalStrength = albedo, normal, [2]float32{scale.X, scale.Y}, strength
	}

	if entry.occlusion != nil {
		draw.AO, draw.AOStrength = entry.occlusion.RuntimeOcclusion()
	}

	if entry.lightmap != nil {
		draw.Lightmap, draw.LightmapScale, draw.LightmapDebug = entry.lightmap.RuntimeLightmap()
	}

	if entry.lightmapEncoding != nil {
		draw.LightmapEncoding, draw.LightmapDiffuse = entry.lightmapEncoding.RuntimeLightmapEncoding()
	}

	if entry.culled != nil {
		draw.CullBackfaces = entry.culled.RuntimeCullBackfaces()
	}

	if entry.rnm != nil {
		if maps := entry.rnm.RuntimeRNMLightmap(); maps != nil {
			draw.RNM, draw.RNMScale, draw.RNMEncoding = maps.Textures, maps.Scale, maps.Encoding
		}
	}

	if entry.source != nil {
		ambient, halfLambert, strength := entry.source.RuntimeSource()

		draw.Source, draw.HalfLambert, draw.SSBumpStrength = true, halfLambert, strength
		for index, color := range ambient {
			draw.AmbientColors[index] = colorArray(color)
		}
	}

	if entry.reflection != nil {
		draw.ReflectionStrength = entry.reflection.RuntimeReflectionStrength()
	}
}

func vectorArray(value Vector3) [3]float32 { return [3]float32{value.X, value.Y, value.Z} }

func colorArray(value Color) [4]float32 {
	if value.A == 0 {
		value.A = 1
	}

	return [4]float32{value.R, value.G, value.B, value.A}
}

func validateRNMLightmap(draw render.DrawCall) error {
	if draw.RNMEncoding > texturedata.RNMRGBE {
		return errInvalidLightmap
	}

	if draw.RNM == ([3]*texturedata.Texture{}) {
		return nil
	}

	if !draw.Source || draw.Prepared == nil || len(draw.Prepared.LightmapUV()) == 0 || draw.RNMScale <= 0 ||
		math.IsNaN(float64(draw.RNMScale)) ||
		math.IsInf(float64(draw.RNMScale), 0) {
		return errInvalidLightmap
	}

	if draw.RNM[0] == nil {
		return errInvalidLightmap
	}

	width, height := 0, 0

	space := texturedata.Space(draw.RNM[0])
	if !validLightmapSpace(space, draw.RNMEncoding, true) {
		return errInvalidLightmap
	}

	for _, image := range draw.RNM {
		if image == nil || texturedata.Space(image) != space {
			return errInvalidLightmap
		}

		w, h := image.Size()
		if width != 0 && (w != width || h != height) {
			return errInvalidLightmap
		}

		width, height = w, h
	}

	return nil
}

func validateRGBLightmap(draw render.DrawCall) error {
	if draw.Lightmap != nil &&
		(!validLightmapSpace(texturedata.Space(draw.Lightmap), draw.LightmapEncoding, draw.LightmapDiffuse) || draw.Prepared == nil || len(draw.Prepared.LightmapUV()) == 0 || draw.LightmapEncoding > texturedata.RNMRGBE) {
		return errInvalidLightmap
	}

	if draw.LightmapDiffuse &&
		(draw.Lightmap == nil || draw.LightmapScale <= 0 || math.IsNaN(float64(draw.LightmapScale)) || math.IsInf(float64(draw.LightmapScale), 0)) {
		return errInvalidLightmap
	}

	return nil
}

// Normalized lightmaps use the texture's existing hardware transfer. RGBE and
// legacy RGB remain numeric linear data; SSBump admission is unchanged.
func validLightmapSpace(space texturedata.ColorSpace, encoding texturedata.RNMEncoding, calibrated bool) bool {
	return space == texturedata.Linear || calibrated && encoding == texturedata.RNMLinear && space == texturedata.SRGB
}
