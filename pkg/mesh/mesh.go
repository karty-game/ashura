// Package mesh provides primitive and custom indexed mesh constructors.
// Meshes use right-handed coordinates with +Y up and counter-clockwise
// triangle winding when viewed from the front. Dimensions are in world units.
// Primitive segment counts default to 24 when zero and otherwise clamp to 3..256.
package mesh

import (
	"errors"
	"fmt"
	"math"

	engine "github.com/karty-game/karty-webgpu/pkg/engine"
)

const (
	defaultSegments    uint32 = 24
	maxSegments        uint32 = 256
	maxVertices               = 1 << 20
	maxIndices                = 3 << 20
	half                      = 2
	minimumSphereRings        = 2
	triangleIndexCount        = 3
)

var (
	errInvalidVertexCount = errors.New("vertex count is outside the supported range")
	errInvalidIndexCount  = errors.New("mesh: index count must be a non-zero multiple of three and within the supported limit")
	errInvalidNormals     = errors.New("mesh: normals must be empty or match the position count")
	errInvalidUVs         = errors.New("mesh: UVs must be empty or match the position count")
	errNonFinitePosition  = errors.New("mesh: position contains a non-finite value")
	errNonFiniteNormal    = errors.New("mesh: normal contains a non-finite value")
	errNonFiniteUV        = errors.New("mesh: UV contains a non-finite value")
	errIndexOutOfRange    = errors.New("mesh: index references a vertex outside the position array")
)

// Data describes indexed triangle geometry. Normals and UVs are
// optional and, when provided, must have one entry per position. Missing normals
// use upward-facing lighting; missing UVs default to zero.
type Data struct {
	Positions []engine.Vector3
	Indices   []uint32
	Normals   []engine.Vector3
	UVs       []engine.Vector2
}

// Mesh contains named geometry that can be attached to a world entity.
// Geometry slices are immutable after attachment. To replace geometry, assign
// newly constructed slices or replace the component with World.Set; in-place
// edits to existing slice elements are unsupported.
//
//nolint:recvcheck // CreateInstance retains a source address; value methods preserve component compatibility.
type Mesh struct {
	Name          string
	Data          Data
	Billboard     bool
	BillboardMode BillboardMode
	BillboardAxis engine.Vector3
}

// Instance is a renderable instance of a source mesh. Add it to a world with
// its own Transform and material components, just like a Mesh.
type Instance struct {
	Name   string
	source *Mesh
}

// BoxOptions sets box dimensions. Zero values default to one world unit.
// Other values are used as provided.
type BoxOptions struct{ Width, Height, Depth float32 }

// SphereOptions sets a UV sphere's diameter and longitudinal segment count.
// Zero values default to diameter 1 and 24 segments. Other dimension values
// are used as provided.
type SphereOptions struct {
	Diameter float32
	Segments uint32
}

// PlaneOptions sets a plane's dimensions in the XZ plane. Zero values default
// to one world unit. Other values are used as provided.
type PlaneOptions struct{ Width, Height float32 }

// BillboardMode controls orientation of an XY quad whose front faces local +Z.
type BillboardMode uint8

const (
	// BillboardSpherical faces the active camera and follows its up direction.
	BillboardSpherical BillboardMode = iota
	// BillboardAxis rotates toward the camera around a fixed world-space axis.
	BillboardAxis
	// BillboardFixed retains the entity's authored transform like an ordinary quad.
	BillboardFixed
)

// BillboardOptions sets world-unit dimensions and the camera-facing mode.
// Zero dimensions select one unit; zero Mode is spherical and zero Axis selects
// world +Y. Axis is used only by BillboardAxis. Unknown modes select spherical.
type BillboardOptions struct {
	Width, Height float32
	Mode          BillboardMode
	Axis          engine.Vector3
}

// CylinderOptions sets a Y-axis cylinder's height, diameter and radial
// segments. Zero values default to one world unit and 24 segments. Other
// dimension values are used as provided.
type CylinderOptions struct {
	Height   float32
	Diameter float32
	Segments uint32
}

// CapsuleOptions sets a Y-axis capsule's total height, diameter and radial
// segments. Height includes the hemispherical ends. Zero values default to
// height 2, diameter 1 and 24 segments. Other dimension values are used as
// provided.
type CapsuleOptions struct {
	Height   float32
	Diameter float32
	Segments uint32
}

// NewCustom validates and copies indexed triangle data. The returned geometry
// owns its slices and is intended to remain immutable after creation.
func NewCustom(name string, data Data) (Mesh, error) {
	if err := validate(data); err != nil {
		return Mesh{}, err
	}

	return Mesh{Name: name, Data: clone(data)}, nil
}

// NewBox returns a centered box with flat normals and independent face UVs.
func NewBox(name string, opts BoxOptions) Mesh {
	width := dimension(opts.Width, 1)
	height := dimension(opts.Height, 1)
	depth := dimension(opts.Depth, 1)

	return Mesh{Name: name, Data: boxData(width, height, depth)}
}

// NewSphere returns a UV sphere centered at the origin with poles on Y.
func NewSphere(name string, opts SphereOptions) Mesh {
	diameter := dimension(opts.Diameter, 1)

	segments := segmentCount(opts.Segments)

	rings := max(segments/half, minimumSphereRings)

	radius := diameter / half
	data := Data{}

	for row := uint32(0); row <= rings; row++ {
		verticalUV := float64(row) / float64(rings)
		phi := verticalUV * math.Pi

		for col := uint32(0); col <= segments; col++ {
			horizontalUV := float64(col) / float64(segments)
			theta := fullTurn * math.Pi * horizontalUV
			surfaceNormal := engine.Vector3{
				X: float32(math.Sin(phi) * math.Cos(theta)),
				Y: float32(math.Cos(phi)),
				Z: float32(math.Sin(phi) * math.Sin(theta)),
			}
			data.Positions = append(
				data.Positions,
				engine.Vector3{X: surfaceNormal.X * radius, Y: surfaceNormal.Y * radius, Z: surfaceNormal.Z * radius},
			)
			data.Normals = append(data.Normals, surfaceNormal)
			data.UVs = append(data.UVs, engine.Vector2{X: float32(horizontalUV), Y: float32(verticalUV)})
		}
	}

	stride := segments + 1

	var rowBase uint32

	for range rings {
		for col := range segments {
			a := rowBase + col
			b := a + stride
			data.Indices = append(data.Indices, a, a+1, b, a+1, b+1, b)
		}

		rowBase += stride
	}

	return Mesh{Name: name, Data: data}
}

// NewPlane returns a centered upward-facing quad in the XZ plane.
func NewPlane(name string, opts PlaneOptions) Mesh {
	width := dimension(opts.Width, 1)
	height := dimension(opts.Height, 1)

	return quad(name, width, height)
}

// NewBillboard returns a world-unit XY quad with local +Z facing the camera.
// Spherical, world-axis constrained and fixed orientation are explicit modes;
// orientation updates change only the transform, never the shared geometry.
func NewBillboard(name string, opts BillboardOptions) Mesh {
	width := dimension(opts.Width, 1)
	height := dimension(opts.Height, 1)

	value := billboard(name, width, height)

	value.BillboardMode, value.BillboardAxis = opts.Mode, opts.Axis
	if value.BillboardMode > BillboardFixed {
		value.BillboardMode = BillboardSpherical
	}

	if value.BillboardAxis == (engine.Vector3{}) {
		value.BillboardAxis = engine.Vector3Up()
	}

	return value
}

// NewCylinder returns a centered cylinder aligned to the Y axis.
func NewCylinder(name string, opts CylinderOptions) Mesh {
	height := dimension(opts.Height, 1)
	diameter := dimension(opts.Diameter, 1)

	segments := segmentCount(opts.Segments)

	return cylinder(name, height, diameter/half, segments)
}

// NewCapsule returns a centered capsule aligned to the Y axis. Height includes
// both hemispherical ends and should be at least the diameter.
func NewCapsule(name string, opts CapsuleOptions) Mesh {
	height := dimension(opts.Height, 2)
	diameter := dimension(opts.Diameter, 1)

	segments := segmentCount(opts.Segments)

	return capsule(name, height, diameter/half, segments)
}

func dimension(value, fallback float32) float32 {
	if value == 0 {
		return fallback
	}

	return value
}

// Zero selects the default; nonzero counts are bounded to avoid accidental
// unbounded allocations in the primitive builders.
func segmentCount(value uint32) uint32 {
	if value == 0 {
		return defaultSegments
	}

	return min(max(value, uint32(3)), maxSegments)
}

func validate(data Data) error {
	if len(data.Positions) == 0 || len(data.Positions) > maxVertices {
		return fmt.Errorf("mesh: %w (valid range is 1 through %d)", errInvalidVertexCount, maxVertices)
	}

	if err := validateIndices(data); err != nil {
		return err
	}

	if err := validateAttributes(data); err != nil {
		return err
	}

	return validateFiniteValues(data)
}

func validateIndices(data Data) error {
	if len(data.Indices) == 0 || len(data.Indices) > maxIndices || len(data.Indices)%triangleIndexCount != 0 {
		return fmt.Errorf("mesh: %w (limit is %d indices)", errInvalidIndexCount, maxIndices)
	}

	for indexPosition, index := range data.Indices {
		if uint64(index) >= uint64(len(data.Positions)) {
			return fmt.Errorf("mesh: index %d, vertex %d: %w", indexPosition, index, errIndexOutOfRange)
		}
	}

	return nil
}

func validateAttributes(data Data) error {
	if len(data.Normals) != 0 && len(data.Normals) != len(data.Positions) {
		return errInvalidNormals
	}

	if len(data.UVs) != 0 && len(data.UVs) != len(data.Positions) {
		return errInvalidUVs
	}

	return nil
}

func validateFiniteValues(data Data) error {
	for index, position := range data.Positions {
		if !finite(position.X) || !finite(position.Y) || !finite(position.Z) {
			return fmt.Errorf("mesh: position %d: %w", index, errNonFinitePosition)
		}
	}

	for index, normal := range data.Normals {
		if !finite(normal.X) || !finite(normal.Y) || !finite(normal.Z) {
			return fmt.Errorf("mesh: normal %d: %w", index, errNonFiniteNormal)
		}
	}

	for index, uv := range data.UVs {
		if !finite(uv.X) || !finite(uv.Y) {
			return fmt.Errorf("mesh: UV %d: %w", index, errNonFiniteUV)
		}
	}

	return nil
}
func clone(d Data) Data {
	return Data{
		Positions: append([]engine.Vector3(nil), d.Positions...),
		Indices:   append([]uint32(nil), d.Indices...),
		Normals:   append([]engine.Vector3(nil), d.Normals...),
		UVs:       append([]engine.Vector2(nil), d.UVs...),
	}
}
func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

// CreateInstance creates a renderable that shares this mesh's geometry.
// The source mesh must remain alive while instances are in use.
func (value *Mesh) CreateInstance(name string) Instance {
	return Instance{Name: name, source: value}
}

// RuntimeMesh provides geometry slices to the engine renderer.
func (value Mesh) RuntimeMesh() ([]engine.Vector3, []uint32, []engine.Vector3, []engine.Vector2, bool) {
	return value.Data.Positions, value.Data.Indices, value.Data.Normals, value.Data.UVs, value.Billboard
}

// RuntimeMeshIdentity marks meshes whose geometry may share one instanced draw.
func (value Instance) RuntimeMeshIdentity() any { return value.source }

// RuntimeMesh exposes the source geometry to the renderer without copying it.
func (value Instance) RuntimeMesh() ([]engine.Vector3, []uint32, []engine.Vector3, []engine.Vector2, bool) {
	if value.source == nil {
		return nil, nil, nil, nil, false
	}

	return value.source.RuntimeMesh()
}

// RuntimeBillboard exposes authored orientation settings to the renderer.
func (value Mesh) RuntimeBillboard() (uint8, engine.Vector3) {
	return uint8(value.BillboardMode), value.BillboardAxis
}

// RuntimeBillboard uses the shared source's orientation mode and world axis.
func (value Instance) RuntimeBillboard() (uint8, engine.Vector3) {
	if value.source == nil {
		return uint8(BillboardFixed), engine.Vector3Up()
	}

	return value.source.RuntimeBillboard()
}
