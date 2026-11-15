package render

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
)

const (
	// PointShadowFaces is the six conventional cubemap views.
	PointShadowFaces = 6
	// PointShadowSampleSize is the sampling uniform byte count.
	PointShadowSampleSize = 32
	// PointShadowFaceSize is the depth-pass matrix and position/range byte count.
	PointShadowFaceSize = 80
)

var (
	errPointRange      = errors.New("point light intensity/range is invalid")
	errPointPosition   = errors.New("point light position is not finite")
	errPointColor      = errors.New("point light color is invalid")
	errPointResolution = errors.New("point shadow resolution must be a power of two from 64 to 2048")
	errPointBias       = errors.New("point shadow bias is invalid")
	errPointCount      = errors.New("first point-shadow slice supports one shadowed point light")
	// ErrPointShadowStartup rejects undeclared or changed runtime shadow resources.
	ErrPointShadowStartup = errors.New("point shadows must be declared before Run; shadow resolution is fixed for the App lifetime")
)

func pointFinite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// ValidatePointLights rejects invalid ranges/resources before GPU calls. This
// first slice has one shadowed point light, independently of other live lights.
func ValidatePointLights(lights []Light) error {
	count := 0

	for _, light := range lights {
		if light.Kind != PointLightKind {
			continue
		}

		if err := validatePoint(light); err != nil {
			return err
		}

		if light.CastShadows && light.Intensity > 0 {
			count++
		}
	}

	if count > 1 {
		return errPointCount
	}

	return nil
}

func validatePoint(light Light) error {
	if !pointFinite(light.Intensity) || light.Intensity < 0 || !pointFinite(light.GroundColor[0]) || light.GroundColor[0] <= .04 {
		return errPointRange
	}

	for _, value := range light.Direction {
		if !pointFinite(value) {
			return errPointPosition
		}
	}

	for _, value := range light.Color {
		if !pointFinite(value) || value < 0 {
			return errPointColor
		}
	}

	if !light.CastShadows || light.Intensity == 0 {
		return nil
	}

	resolution := light.ShadowResolution
	if resolution < 64 || resolution > 2048 || resolution&(resolution-1) != 0 {
		return errPointResolution
	}

	if !pointFinite(light.ShadowBias) || light.ShadowBias < 0 {
		return errPointBias
	}

	return nil
}

// ShadowLight selects the admitted active shadow source.
func ShadowLight(frame Frame) (Light, bool) {
	for _, light := range frame.Lights {
		if light.Kind == PointLightKind && light.CastShadows && light.Intensity > 0 {
			return light, true
		}
	}

	return Light{}, false
}

type shadowCaster struct {
	id                      uint32
	prepared                *meshdata.Geometry
	vertices                *Vertex
	indices                 *uint32
	vertexCount, indexCount int
	transform               [16]float32
	version                 uint64
	instances               int
	cull                    bool
}

// CastsPointShadow selects opaque mesh casters. Standard cutouts are excluded
// until the shadow pass supports their alpha coverage; solid-card shadows would
// otherwise disagree with the visible surface.
func CastsPointShadow(draw DrawCall) bool {
	return HasGeometry(draw) && draw.Impostor == nil && !draw.AlphaMask
}

// PointShadowState compares retained draw metadata, never vertex/index payloads.
// Camera, light color/intensity and unrelated material edits do not dirty depth.
type PointShadowState struct {
	position   [3]float32
	rangeValue float32
	resolution uint32
	ready      bool
	casters    []shadowCaster
}

// Update reports whether the shadow depth must be regenerated this submission.
func (state *PointShadowState) Update(frame Frame, light Light) bool {
	dirty := !state.ready || state.position != light.Direction || state.rangeValue != light.GroundColor[0] ||
		state.resolution != light.ShadowResolution
	count := 0

	for _, draw := range frame.Draws {
		if !CastsPointShadow(draw) {
			continue
		}

		key := shadowCaster{
			id:          draw.ID,
			prepared:    draw.Prepared,
			vertexCount: len(draw.Vertices),
			indexCount:  IndexCount(draw),
			transform:   draw.Transform,
			version:     draw.InstanceVersion,
			instances:   len(draw.Instances),
			cull:        draw.CullBackfaces,
		}
		if len(draw.Vertices) > 0 {
			key.vertices = &draw.Vertices[0]
		}

		if len(draw.Indices) > 0 {
			key.indices = &draw.Indices[0]
		}

		if count < len(state.casters) {
			if state.casters[count] != key {
				dirty = true
			}

			state.casters[count] = key
		} else {
			state.casters = append(state.casters, key)
			dirty = true
		}

		count++
	}

	if count != len(state.casters) {
		clear(state.casters[count:])
		state.casters = state.casters[:count]
		dirty = true
	}

	state.position, state.rangeValue, state.resolution = light.Direction, light.GroundColor[0], light.ShadowResolution
	state.ready = true

	return dirty
}

// Invalidate ensures recreated resources receive a complete six-face render.
func (state *PointShadowState) Invalidate() { state.ready = false }

// PointShadowSampleUniform contains position/range, world-space comparison bias
// and an enabled flag. It is shared by all Standard material pipeline variants.
func PointShadowSampleUniform(light Light, enabled bool) [PointShadowSampleSize]byte {
	values := [8]float32{light.Direction[0], light.Direction[1], light.Direction[2], light.GroundColor[0], light.ShadowBias, 0, 0, 0}
	if enabled {
		values[5] = 1
	}

	var data [PointShadowSampleSize]byte
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}

	return data
}

// PointShadowFaceUniform uses WebGPU's cube convention (+X,-X,+Y,-Y,+Z,-Z).
// The fragment shader stores radial distance/range instead of projected depth.
func PointShadowFaceUniform(light Light, face int) [PointShadowFaceSize]byte {
	directions := [6][3]float32{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	ups := [6][3]float32{{0, -1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}, {0, -1, 0}, {0, -1, 0}}
	direction, up := directions[face], ups[face]
	cross := func(a, b [3]float32) [3]float32 {
		return [3]float32{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
	}
	right := cross(direction, up)
	// Look-at RH rows: right, corrected up, -forward.
	corrected := cross(right, direction)

	var view [16]float32
	for i := range 3 {
		view[i*4] = right[i]
		view[i*4+1] = corrected[i]
		view[i*4+2] = -direction[i]
	}

	view[15] = 1
	for row := range 3 {
		view[12+row] = -(view[row]*light.Direction[0] + view[4+row]*light.Direction[1] + view[8+row]*light.Direction[2])
	}

	near, far := float32(.02), light.GroundColor[0]
	projection := [16]float32{0: 1, 5: -1, 10: far / (near - far), 11: -1, 14: near * far / (near - far)}

	var result [20]float32

	for column := range 4 {
		for row := range 4 {
			for inner := range 4 {
				result[column*4+row] += projection[inner*4+row] * view[column*4+inner]
			}
		}
	}

	copy(result[16:19], light.Direction[:])
	result[19] = far

	var data [PointShadowFaceSize]byte
	for i, v := range result {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}

	return data
}
