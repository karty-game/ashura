package engine

import "github.com/karty-game/karty-webgpu/internal/render"

// ProjectionKind selects a camera projection.
type ProjectionKind uint8

const (
	PerspectiveProjection  ProjectionKind = ProjectionKind(render.PerspectiveProjection)
	OrthographicProjection ProjectionKind = ProjectionKind(render.OrthographicProjection)
)

// ProjectionData describes a camera without positional tuple fields. Perspective
// uses FieldOfViewDegrees; orthographic uses ViewHeight. Near and Far are world
// distances. The viewport supplies the aspect ratio.
type ProjectionData struct {
	Kind               ProjectionKind
	FieldOfViewDegrees float32
	ViewHeight         float32
	Near, Far          float32
}

// ProjectionProvider is the camera extension contract for application components.
// World stores a component by value and calls the stored component on the App
// loop. Direct field changes through Get remain visible. A component implementing
// this and the legacy RuntimeProjection method uses ProjectionData.
type ProjectionProvider interface {
	ProjectionData() ProjectionData
}

// LightKind selects the meaning of a light descriptor's direction and colors.
type LightKind uint32

const (
	HemisphereLight  LightKind = LightKind(render.HemisphereLightKind)
	DirectionalLight LightKind = LightKind(render.DirectionalLightKind)
	PointLight       LightKind = LightKind(render.PointLightKind)
)

// PointShadow configures the existing point-light shadow feature. Enabled and
// Resolution must be declared before Run; resolution is fixed for the App
// lifetime. Bias is a world-space distance. DebugFaces enables the debug overlay.
type PointShadow struct {
	Enabled    bool
	Resolution uint32
	Bias       float32
	DebugFaces bool
}

// LightData describes a light with named fields and linear colors. Hemisphere
// uses Direction, Color (sky) and GroundColor. Directional uses Direction and
// Color. Point uses Position, Range and Shadow; it does not use GroundColor.
// Directional Direction follows light rays toward illuminated surfaces.
type LightData struct {
	Kind        LightKind
	Direction   Vector3
	Position    Vector3
	Color       Color
	GroundColor Color
	Intensity   float32
	Range       float32
	Shadow      PointShadow
}

// LightProvider is the light extension contract for application components.
// Descriptor values are read on the App loop; they must not create resources or
// perform asset processing. A component implementing this and the legacy
// RuntimeLight method uses LightData, including its point-shadow settings.
type LightProvider interface {
	LightData() LightData
}

type projectionBinding struct {
	typed  ProjectionProvider
	legacy cameraComponent
}

func (binding projectionBinding) valid() bool { return binding.typed != nil || binding.legacy != nil }

func (binding projectionBinding) read() ProjectionData {
	if binding.typed != nil {
		return binding.typed.ProjectionData()
	}

	kind, fov, height, near, far := binding.legacy.RuntimeProjection()

	return ProjectionData{Kind: ProjectionKind(kind), FieldOfViewDegrees: fov, ViewHeight: height, Near: near, Far: far}
}

type lightBinding struct {
	typed  LightProvider
	legacy lightComponent
	shadow pointShadowComponent
	debug  pointShadowDebugComponent
}

func (binding lightBinding) renderLight() render.Light {
	if binding.typed == nil {
		kind, direction, color, ground, intensity := binding.legacy.RuntimeLight()

		value := render.Light{Kind: kind, Direction: direction, Color: color, GroundColor: ground, Intensity: intensity}
		if binding.shadow != nil {
			value.CastShadows, value.ShadowResolution, value.ShadowBias = binding.shadow.RuntimePointShadow()
		}

		if binding.debug != nil {
			value.DebugShadowFaces = binding.debug.RuntimePointShadowDebug()
		}

		return value
	}

	data := binding.typed.LightData()

	value := render.Light{
		Kind: uint32(data.Kind), Direction: vectorArray(data.Direction),
		Color:       [3]float32{data.Color.R, data.Color.G, data.Color.B},
		GroundColor: [3]float32{data.GroundColor.R, data.GroundColor.G, data.GroundColor.B},
		Intensity:   data.Intensity,
	}
	if data.Kind == PointLight {
		value.Direction = vectorArray(data.Position)

		value.GroundColor = [3]float32{data.Range, 0, 0}
		if data.Shadow.Enabled && data.Intensity > 0 {
			value.GroundColor[1] = 1
		}

		value.CastShadows = data.Shadow.Enabled
		value.ShadowResolution = data.Shadow.Resolution
		value.ShadowBias = data.Shadow.Bias
		value.DebugShadowFaces = data.Shadow.DebugFaces
	}

	return value
}
