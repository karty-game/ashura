package engine

// Vector2 is a two-dimensional vector.
type Vector2 struct {
	X float32
	Y float32
}

// Vector3 is a three-dimensional vector.
type Vector3 struct {
	X float32
	Y float32
	Z float32
}

// Vector3Zero returns the zero vector.
func Vector3Zero() Vector3 { return Vector3{} }

// Vector3One returns a vector with all components set to one.
func Vector3One() Vector3 { return Vector3{X: 1, Y: 1, Z: 1} }

// Vector3Up returns the positive Y axis.
func Vector3Up() Vector3 { return Vector3{Y: 1} }

// Vector3Down returns the negative Y axis.
func Vector3Down() Vector3 { return Vector3{Y: -1} }

// Vector3Left returns the negative X axis.
func Vector3Left() Vector3 { return Vector3{X: -1} }

// Vector3Right returns the positive X axis.
func Vector3Right() Vector3 { return Vector3{X: 1} }

// Vector3Forward returns the negative Z axis, following the engine's
// right-handed coordinate system.
func Vector3Forward() Vector3 { return Vector3{Z: -1} }

// Vector3Backward returns the positive Z axis, following the engine's
// right-handed coordinate system.
func Vector3Backward() Vector3 { return Vector3{Z: 1} }

// Color stores linear red, green, blue, and alpha channels.
type Color struct {
	R float32
	G float32
	B float32
	A float32
}

const (
	linearGreen      float32 = 0.216
	linearOrange     float32 = 0.376
	linearPinkGreen  float32 = 0.527
	linearPinkBlue   float32 = 0.623
	linearBrownRed   float32 = 0.381
	linearBrownGreen float32 = 0.023
	linearLightGray  float32 = 0.645
	linearDarkGray   float32 = 0.402
)

// The named color functions return common colors with linear RGB channels.
// Functions return values so callers cannot mutate shared package state.

// ColorBlack returns opaque black.
func ColorBlack() Color { return Color{A: 1} }

// ColorWhite returns opaque white.
func ColorWhite() Color { return Color{R: 1, G: 1, B: 1, A: 1} }

// ColorRed returns opaque red.
func ColorRed() Color { return Color{R: 1, A: 1} }

// ColorGreen returns opaque green.
func ColorGreen() Color { return Color{G: linearGreen, A: 1} }

// ColorBlue returns opaque blue.
func ColorBlue() Color { return Color{B: 1, A: 1} }

// ColorYellow returns opaque yellow.
func ColorYellow() Color { return Color{R: 1, G: 1, A: 1} }

// ColorOrange returns opaque orange.
func ColorOrange() Color { return Color{R: 1, G: linearOrange, A: 1} }

// ColorPurple returns opaque purple.
func ColorPurple() Color { return Color{R: linearGreen, B: linearGreen, A: 1} }

// ColorCyan returns opaque cyan.
func ColorCyan() Color { return Color{G: 1, B: 1, A: 1} }

// ColorMagenta returns opaque magenta.
func ColorMagenta() Color { return Color{R: 1, B: 1, A: 1} }

// ColorPink returns opaque pink.
func ColorPink() Color { return Color{R: 1, G: linearPinkGreen, B: linearPinkBlue, A: 1} }

// ColorBrown returns opaque brown.
func ColorBrown() Color {
	return Color{R: linearBrownRed, G: linearBrownGreen, B: linearBrownGreen, A: 1}
}

// ColorGray returns opaque gray.
func ColorGray() Color { return Color{R: linearGreen, G: linearGreen, B: linearGreen, A: 1} }

// ColorLightGray returns opaque light gray.
func ColorLightGray() Color {
	return Color{R: linearLightGray, G: linearLightGray, B: linearLightGray, A: 1}
}

// ColorDarkGray returns opaque dark gray.
func ColorDarkGray() Color {
	return Color{R: linearDarkGray, G: linearDarkGray, B: linearDarkGray, A: 1}
}

// ColorLime returns opaque lime.
func ColorLime() Color { return Color{G: 1, A: 1} }

// Transform describes an entity's position, rotation, and scale. Rotation is
// expressed in radians around the X, Y, and Z axes.
type Transform struct {
	Position Vector3
	Rotation Vector3
	Scale    Vector3
}

// DefaultTransform returns a transform with unit scale.
func DefaultTransform() Transform { return Transform{Scale: Vector3One()} }

// MouseButton identifies a pointer button. Touch drags have separate input deltas.
type MouseButton uint8

const (
	MouseButtonLeft MouseButton = iota
	MouseButtonMiddle
	MouseButtonRight
)

// Key identifies a keyboard key used by the engine input API.
type Key uint8

const (
	Key1     Key = '1'
	Key2     Key = '2'
	Key3     Key = '3'
	KeyA     Key = 'A'
	KeyB     Key = 'B'
	KeyH     Key = 'H'
	KeyR     Key = 'R'
	KeyD     Key = 'D'
	KeyN     Key = 'N'
	KeyO     Key = 'O'
	KeyS     Key = 'S'
	KeyW     Key = 'W'
	KeySpace Key = ' '
	KeyShift Key = 0x80
)
