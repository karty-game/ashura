// Package reflectionscene owns the scene types shared by generated runtime data and the offline bake.
// Geometry uses centered primitives, Euler rotations and unit scale.
package reflectionscene

// Vector3 describes authored positions, dimensions and Euler rotations.
type Vector3 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// Color stores authored linear RGBA channels.
type Color struct {
	R float32 `json:"r"`
	G float32 `json:"g"`
	B float32 `json:"b"`
	A float32 `json:"a"`
}

// Hemisphere describes the scene's ambient sky/ground fill.
type Hemisphere struct {
	Name        string  `json:"name"`
	Direction   Vector3 `json:"direction"`
	SkyColor    Color   `json:"skyColor"`
	GroundColor Color   `json:"groundColor"`
	Intensity   float32 `json:"intensity"`
}

// Directional describes rays traveling from the source toward the surfaces.
type Directional struct {
	Name      string  `json:"name"`
	Direction Vector3 `json:"direction"`
	Color     Color   `json:"color"`
	Intensity float32 `json:"intensity"`
}

// Shape selects the primitive used by both the visible scene and its panorama.
type Shape uint8

const (
	Plane Shape = iota
	Box
	Sphere
)

// SphereSegments controls the visible orb's tessellation.
const SphereSegments = 32

// Surface describes the material independently of any loaded GPU resources.
type Surface struct {
	Color              Color   `json:"color"`
	Roughness          float32 `json:"roughness"`
	ReflectionStrength float32 `json:"reflectionStrength"`
}

// Object places one primitive. Planes use Size.X/Z; spheres use Size.X as diameter.
type Object struct {
	Name     string  `json:"name"`
	Shape    Shape   `json:"shape"`
	Size     Vector3 `json:"size"`
	Position Vector3 `json:"position"`
	Rotation Vector3 `json:"rotation"`
	Surface  Surface `json:"surface"`
}

// Scene contains all geometry, lighting and probe placement used in this sample.
type Scene struct {
	Objects          []Object    `json:"objects,omitempty"`
	Fill             Hemisphere  `json:"fill"`
	Key              Directional `json:"key"`
	ProbePosition    Vector3     `json:"probePosition"`
	ProbeHalfExtents Vector3     `json:"probeHalfExtents"`
}
