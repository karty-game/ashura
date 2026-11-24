package render

import (
	"errors"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/meshdata"
)

const (
	// PointShadowFaces is the six directional views of a point light.
	PointShadowFaces = 6
	// PointShadowSampleSize is the sampling uniform byte count.
	PointShadowSampleSize = pointShadowHeaderBytes + PointShadowFaces*SpotMatrixBytes
	// PointShadowFaceSize is the depth-pass matrix byte count.
	PointShadowFaceSize     = SpotMatrixBytes
	pointShadowHeaderFloats = 8
	pointShadowHeaderBytes  = pointShadowHeaderFloats * 4
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

type ShadowCasterKey struct {
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

// PointShadowState retains physical face masks across camera-only changes.
type PointShadowState struct {
	position   [3]float32
	rangeValue float32
	resolution uint32
	ready      bool
	revision   uint64
	camera     shadowCamera
	casters    []ShadowCasterKey
	faceMasks  []uint8
	fixed      [PointShadowFaces][16]float32
	views      [PointShadowFaces][16]float32
	sampleData [PointShadowSampleSize]byte
	header     [pointShadowHeaderFloats]float32
}

// FaceMasks returns the retained draw-indexed masks for the six physical faces.
func (state *PointShadowState) FaceMasks() []uint8 { return state.faceMasks }

// Update returns the bitmask of faces whose projected depth is stale.
func (state *PointShadowState) Update(frame Frame, light Light) uint8 {
	moved := !state.ready || state.position != light.Direction || state.rangeValue != light.GroundColor[0] ||
		state.resolution != light.ShadowResolution
	camera := cameraForShadows(frame)

	var dirty uint8

	if moved {
		dirty = allShadowFaces

		for face := range state.fixed {
			state.fixed[face] = pointShadowFaceMatrix(light, face)
		}
	}

	if moved || camera != state.camera {
		for face, fixed := range state.fixed {
			view := shadowProjection(camera, light.Direction, fixed)
			if state.views[face] != view {
				offset := pointShadowHeaderBytes + face*SpotMatrixBytes
				encodeUniform(state.sampleData[offset:offset+SpotMatrixBytes], view[:])
				state.views[face] = view
				dirty |= 1 << face
			}
		}

		state.camera = camera
	}

	if moved || frame.ShadowCasterRevision == 0 || frame.ShadowCasterRevision != state.revision {
		dirty |= state.updateCasters(frame.Draws, light, moved)
		state.revision = frame.ShadowCasterRevision
	}

	state.position, state.rangeValue, state.resolution = light.Direction, light.GroundColor[0], light.ShadowResolution
	state.ready = true

	return dirty
}

// CasterKey compares retained geometry and instances without reading their payloads.
func CasterKey(draw DrawCall) ShadowCasterKey {
	if !CastsPointShadow(draw) {
		return ShadowCasterKey{}
	}

	key := ShadowCasterKey{
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

	return key
}

// Invalidate ensures recreated resources receive a complete six-face render.
func (state *PointShadowState) Invalidate() { state.ready = false }

// SampleUniform packs the same face matrices used to record the depth passes.
func (state *PointShadowState) SampleUniform(light Light, enabled bool) [PointShadowSampleSize]byte {
	if !enabled {
		return [PointShadowSampleSize]byte{}
	}

	header := [pointShadowHeaderFloats]float32{
		light.Direction[0], light.Direction[1], light.Direction[2], light.GroundColor[0], light.ShadowBias, 1,
	}
	if header != state.header {
		encodeUniform(state.sampleData[:pointShadowHeaderBytes], header[:])
		state.header = header
	}

	return state.sampleData
}

func (state *PointShadowState) FaceUniform(face int) [PointShadowFaceSize]byte {
	offset := pointShadowHeaderBytes + face*SpotMatrixBytes

	return [PointShadowFaceSize]byte(state.sampleData[offset : offset+PointShadowFaceSize])
}

func (state *PointShadowState) updateCasters(draws []DrawCall, light Light, moved bool) uint8 {
	count := len(draws)
	oldCount := len(state.casters)

	var dirty uint8

	if count < oldCount {
		for _, mask := range state.faceMasks[count:] {
			dirty |= mask
		}

		clear(state.casters[count:])
	}

	state.casters = slices.Grow(state.casters, max(0, count-oldCount))[:count]

	state.faceMasks = slices.Grow(state.faceMasks, max(0, count-oldCount))[:count]
	if count > oldCount {
		clear(state.faceMasks[oldCount:])
	}

	for index, draw := range draws {
		key := CasterKey(draw)

		changed := index >= oldCount || state.casters[index] != key
		if !changed && !moved {
			continue
		}

		oldMask := state.faceMasks[index]
		state.faceMasks[index] = 0
		state.casters[index] = key

		if key != (ShadowCasterKey{}) {
			state.faceMasks[index] = PointShadowFaceMask(draw, light)
		}

		dirty |= oldMask | state.faceMasks[index]
	}

	return dirty
}

const pointFaceGuard = 1.04

// Each face covers one dominant world direction, with overlapping edges
// for bilinear depth comparison where the receiver crosses onto another face.
func pointShadowFaceMatrix(light Light, face int) [16]float32 {
	directions := [6][3]float32{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	ups := [6][3]float32{{0, -1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}, {0, -1, 0}, {0, -1, 0}}
	forward := directions[face]
	right := cross3(forward, ups[face])
	lightUp := cross3(forward, right)

	near, far := float32(.02), light.GroundColor[0]
	depthScale := far / (far - near)
	scale := float32(1.0 / pointFaceGuard)

	var result [16]float32
	for axis := range 3 {
		result[axis*4] = right[axis] * scale
		result[axis*4+1] = lightUp[axis] * scale
		result[axis*4+2] = forward[axis] * depthScale
		result[axis*4+3] = forward[axis]
	}

	result[12] = -dot3(right, light.Direction) * scale
	result[13] = -dot3(lightUp, light.Direction) * scale
	result[14] = (-dot3(forward, light.Direction) - near) * depthScale
	result[15] = -dot3(forward, light.Direction)

	return result
}
