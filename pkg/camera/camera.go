// Package camera defines projection and navigation components for camera
// entities. Values can be attached directly with World.Add.
package camera

import (
	"github.com/karty-game/karty-webgpu/internal/render"
	engine "github.com/karty-game/karty-webgpu/pkg/engine"
)

const (
	defaultFieldOfView     = 60
	defaultNear            = 0.1
	defaultFar             = 1000
	defaultMoveSpeed       = 5
	defaultLookSensitivity = 0.2
	defaultViewHeight      = 20
	defaultZoomSpeed       = 0.1
	minimumMapZoom         = 0.1
	maximumMapZoom         = 100
	defaultOrbitDistance   = 8
	maximumOrbitDistance   = 1000
)

// Perspective describes a perspective projection. FieldOfViewDegrees is the
// vertical field of view in degrees. The renderer derives aspect from viewport.
type Perspective struct {
	FieldOfViewDegrees float32
	Near               float32
	Far                float32
}

// Orthographic describes an orthographic projection. ViewHeight is the visible
// vertical world-space extent; the renderer derives horizontal extent from the
// viewport.
type Orthographic struct {
	ViewHeight float32
	Near       float32
	Far        float32
}

// FreeControls describes first-person movement and pointer-look settings. MoveSpeed is in world units per second; LookSensitivity is in degrees per pointer pixel.
type FreeControls struct {
	MoveSpeed       float32
	LookSensitivity float32
}

// OrbitControls rotates around Target using a primary drag or one finger.
// Perspective zoom changes Distance; orthographic zoom changes ViewHeight.
// MinDistance/MaxDistance bound the distance or view height in world units.
// RotateSpeed is degrees per logical pixel; ZoomSpeed is logarithmic per wheel step.
// A two-finger drag pans Target in the view plane; spreading fingers zooms in.
// Zero settings select defaults. A zero Distance derives the initial pose from
// Transform.Position, or uses eight units when the camera is at Target.
type OrbitControls struct {
	RotateButton engine.MouseButton
	Target       engine.Vector3
	Distance     float32
	MinDistance  float32
	MaxDistance  float32
	RotateSpeed  float32
	ZoomSpeed    float32
}

// PanZoomControls pans an orthographic camera in its view plane using a primary
// drag or touch. PanSpeed scales the pixel-to-world motion. MinZoom/MaxZoom bound
// the projection's ViewHeight in world units. Wheel steps and pinch gestures zoom.
// Zero settings select defaults; perspective cameras ignore this component.
type PanZoomControls struct {
	DragButton engine.MouseButton
	PanSpeed   float32
	ZoomSpeed  float32
	MinZoom    float32
	MaxZoom    float32
}

// FirstPerson returns a useful perspective projection and free-look controls.
// Attach both values to the same camera entity.
func FirstPerson() (Perspective, FreeControls) {
	return Perspective{FieldOfViewDegrees: defaultFieldOfView, Near: defaultNear, Far: defaultFar}, FreeControls{
		MoveSpeed:       defaultMoveSpeed,
		LookSensitivity: defaultLookSensitivity,
	}
}

// RuntimeProjection provides perspective settings to the engine renderer.
func (projection Perspective) RuntimeProjection() (uint8, float32, float32, float32, float32) {
	return render.PerspectiveProjection, projection.FieldOfViewDegrees, 0, projection.Near, projection.Far
}

// MapView returns a useful orthographic projection and pan/zoom controls.
func MapView() (Orthographic, PanZoomControls) {
	return Orthographic{ViewHeight: defaultViewHeight, Near: defaultNear, Far: defaultFar}, PanZoomControls{
		PanSpeed:  1,
		ZoomSpeed: defaultZoomSpeed,
		MinZoom:   minimumMapZoom,
		MaxZoom:   maximumMapZoom,
	}
}

// RuntimeProjection provides orthographic settings to the engine renderer.
func (projection *Orthographic) RuntimeProjection() (uint8, float32, float32, float32, float32) {
	return render.OrthographicProjection, 0, projection.ViewHeight, projection.Near, projection.Far
}

// RuntimeFreeControls provides first-person control settings to the engine loop.
func (controls FreeControls) RuntimeFreeControls() (float32, float32) {
	return controls.MoveSpeed, controls.LookSensitivity
}

// Orbit returns a perspective projection and target-centered orbit controls.
// Attach both to the same entity with a Transform. Its rotation sets the initial
// view angles; controls.Target and Distance determine the camera position.
func Orbit() (Perspective, OrbitControls) {
	projection, _ := FirstPerson()

	return projection, OrbitControls{
		Distance: defaultOrbitDistance, MinDistance: minimumMapZoom, MaxDistance: maximumOrbitDistance,
		RotateSpeed: defaultLookSensitivity, ZoomSpeed: defaultZoomSpeed,
	}
}

// RuntimeOrbitControls connects stored navigation state to the engine loop.
func (controls *OrbitControls) RuntimeOrbitControls() (*engine.Vector3, *float32, float32, float32, float32, float32, engine.MouseButton) {
	return &controls.Target, &controls.Distance, controls.MinDistance, controls.MaxDistance, controls.RotateSpeed, controls.ZoomSpeed, controls.RotateButton
}

// RuntimePanZoomControls supplies planar navigation settings to the engine loop.
func (controls PanZoomControls) RuntimePanZoomControls() (float32, float32, float32, float32, engine.MouseButton) {
	return controls.PanSpeed, controls.ZoomSpeed, controls.MinZoom, controls.MaxZoom, controls.DragButton
}

// RuntimeViewHeight allows navigation to update the stored orthographic extent.
func (projection *Orthographic) RuntimeViewHeight() *float32 { return &projection.ViewHeight }
