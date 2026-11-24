package render

import (
	"errors"
	"math"
	"slices"
)

const SpotShadowCount = 2
const spotMatrixFloats = 16
const SpotMatrixBytes = spotMatrixFloats * 4

// The filtered comparison covers neighboring texels. Offset stored depth
// along the rasterized depth gradient to reduce false self-shadowing.
const (
	ShadowDepthBias           = 2
	ShadowDepthBiasSlopeScale = 2.0
)

// SpotShadowResolution is the common edge length for both retained spotlight maps.
const SpotShadowResolution = 768

var (
	errSpotFeature  = errors.New("spotlights require the clustered lighting feature")
	errSpotSettings = errors.New("invalid spotlight position, direction, range, intensity, color or cone")
	errSpotCount    = errors.New("clustered lighting supports two declared shadowed spotlights")
)

func ValidateSpots(lights []Light, clustered bool) error {
	count := 0

	for _, light := range lights {
		if light.Kind != SpotLightKind {
			continue
		}

		if !clustered {
			return errSpotFeature
		}

		if err := validatePoint(
			Light{Direction: light.Direction, GroundColor: light.GroundColor, Color: light.Color, Intensity: light.Intensity},
		); err != nil {
			return errSpotSettings
		}

		if normalized3(light.SpotDirection) == ([3]float32{}) || !pointFinite(light.InnerAngle) || !pointFinite(light.OuterAngle) ||
			light.InnerAngle < 0 || light.OuterAngle >= math.Pi/2 || light.OuterAngle <= light.InnerAngle ||
			cosine(light.InnerAngle) <= cosine(light.OuterAngle) {
			return errSpotSettings
		}

		if light.CastShadows {
			count++
		}
	}

	if count > SpotShadowCount {
		return errSpotCount
	}

	return nil
}

type SpotView struct {
	Matrix [16]float32
	Active bool
}

type spotPose struct {
	position, direction [3]float32
	radius, angle       float32
}

type spotCaster struct {
	key  ShadowCasterKey
	mask uint8
}

// SpotShadowState compares only retained caster metadata. Bounds are tested
// when the corresponding light or a caster changes.
type SpotShadowState struct {
	Views        [SpotShadowCount]SpotView
	poses        [SpotShadowCount]spotPose
	fixed        [SpotShadowCount][16]float32
	camera       shadowCamera
	casters      []spotCaster
	masks        []uint8
	lastRevision uint64
}

func (state *SpotShadowState) Update(frame Frame) (uint8, []uint8) {
	active, moved := state.updatePoses(frame.Lights)
	if active == 0 {
		state.lastRevision = 0
		state.Views = [SpotShadowCount]SpotView{}
		clear(state.casters)
		state.casters = state.casters[:0]

		return 0, nil
	}

	dirty := state.updateViews(cameraForShadows(frame), active, moved)

	if moved != 0 || frame.ShadowCasterRevision == 0 || frame.ShadowCasterRevision != state.lastRevision {
		state.lastRevision = frame.ShadowCasterRevision
		dirty |= state.updateCasters(frame.Draws, active, moved)

		state.masks = slices.Grow(state.masks[:0], len(frame.Draws))[:len(frame.Draws)]
		for index, caster := range state.casters {
			state.masks[index] = caster.mask
		}
	}

	if dirty == 0 {
		return 0, nil
	}

	return dirty, state.masks
}

func (state *SpotShadowState) updateViews(camera shadowCamera, active, moved uint8) uint8 {
	var dirty uint8

	for slot, pose := range state.poses {
		bit := uint8(1 << slot)
		if active&bit == 0 {
			state.Views[slot] = SpotView{}

			continue
		}

		if moved&bit != 0 || state.camera != camera {
			matrix := shadowProjection(camera, pose.position, state.fixed[slot])
			if matrix != state.Views[slot].Matrix {
				dirty |= bit
			}

			state.Views[slot] = SpotView{Matrix: matrix, Active: true}
		}
	}

	state.camera = camera

	return dirty
}

func (state *SpotShadowState) updateCasters(draws []DrawCall, active, moved uint8) uint8 {
	dirty := moved & active

	oldCount := len(state.casters)
	if oldCount < len(draws) {
		state.casters = slices.Grow(state.casters, len(draws)-oldCount)[:len(draws)]
		clear(state.casters[oldCount:])
	}

	for index, draw := range draws {
		caster := &state.casters[index]
		key := CasterKey(draw)

		changed := key != caster.key
		if !changed && moved == 0 {
			continue
		}

		oldMask := caster.mask

		recheck := moved & active
		if changed {
			recheck = active
		}

		caster.mask &= active &^ recheck

		if key != (ShadowCasterKey{}) {
			for slot, matrix := range state.fixed {
				if recheck&(1<<slot) != 0 && spotCasterVisible(draw, matrix) {
					caster.mask |= 1 << slot
				}
			}
		}

		if changed {
			dirty |= (oldMask | caster.mask) & active
		}

		caster.key = key
	}

	for index := len(draws); index < oldCount; index++ {
		dirty |= state.casters[index].mask & active
	}

	clear(state.casters[len(draws):])
	state.casters = state.casters[:len(draws)]

	return dirty
}

func (state *SpotShadowState) updatePoses(lights []Light) (active, moved uint8) {
	var poses [SpotShadowCount]spotPose

	slot := 0

	for _, light := range lights {
		if light.Kind != SpotLightKind || !light.CastShadows {
			continue
		}

		if light.Intensity > 0 {
			poses[slot] = spotPose{light.Direction, normalized3(light.SpotDirection), light.GroundColor[0], light.OuterAngle}
			active |= 1 << slot
		}

		slot++
	}

	for slot, pose := range poses {
		if pose != state.poses[slot] {
			moved |= 1 << slot

			if active&(1<<slot) != 0 {
				state.fixed[slot] = spotMatrix(pose)
			}
		}
	}

	state.poses = poses

	return active, moved
}

func spotCasterVisible(draw DrawCall, matrix [16]float32) bool {
	bounds := draw.localBounds()
	if !bounds.Valid || draw.Billboard || len(draw.Instances) > maxShadowCullInstances {
		return true
	}

	// A convex box can be rejected only if all eight corners are outside the
	// same clip plane. Geometry whose bounds are unknown stays in the pass.
	for index := range max(1, len(draw.Instances)) {
		model := instanceAt(draw, index).Model
		if model[3] != 0 || model[7] != 0 || model[11] != 0 || model[15] != 1 {
			return true
		}

		outside := uint8(0x3f)

		for corner := range 8 {
			point := bounds.Minimum
			for axis := range point {
				if corner&(1<<axis) != 0 {
					point[axis] = bounds.Maximum[axis]
				}
			}

			world := transformPoint(model, point)
			clip := transformPoint(matrix, world)
			w := matrix[3]*world[0] + matrix[7]*world[1] + matrix[11]*world[2] + matrix[15]

			planes := [6]float32{w + clip[0], w - clip[0], w + clip[1], w - clip[1], clip[2], w - clip[2]}
			for plane, value := range planes {
				const epsilon = 1e-4
				if !pointFinite(value) || value >= -epsilon*(absolute(w)+1) {
					outside &^= 1 << plane
				}
			}
		}

		if outside == 0 {
			return true
		}
	}

	return false
}

func spotMatrix(pose spotPose) [16]float32 {
	forward := pose.direction

	const poleThreshold = 0.98

	worldUp := [3]float32{0, 1, 0}
	if absolute(forward[1]) > poleThreshold {
		worldUp = [3]float32{0, 0, 1}
	}

	right := normalized3(cross3(forward, worldUp))
	lightUp := cross3(right, forward)
	near, far := max(float32(.02), pose.radius/1024), pose.radius

	// Match the engine's WebGPU clip-space convention: positive clip W is
	// distance along the light direction, with depth from zero to one.
	scale := float32(1 / math.Tan(float64(pose.angle)))
	depthScale := far / (far - near)

	var result [16]float32
	for axis := range 3 {
		result[axis*4] = right[axis] * scale
		result[axis*4+1] = lightUp[axis] * scale
		result[axis*4+2] = forward[axis] * depthScale
		result[axis*4+3] = forward[axis]
	}

	result[12] = -dot3(right, pose.position) * scale
	result[13] = -dot3(lightUp, pose.position) * scale
	result[14] = (-dot3(forward, pose.position) - near) * depthScale
	result[15] = -dot3(forward, pose.position)

	return result
}

func (view SpotView) Uniform() [SpotMatrixBytes]byte {
	var data [SpotMatrixBytes]byte
	encodeUniform(data[:], view.Matrix[:])

	return data
}

func cosine(angle float32) float32 { return float32(math.Cos(float64(angle))) }

func dot3(a, b [3]float32) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross3(a, b [3]float32) [3]float32 {
	return [3]float32{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func normalized3(value [3]float32) [3]float32 {
	axisX, axisY, axisZ := float64(value[0]), float64(value[1]), float64(value[2])

	length := math.Sqrt(axisX*axisX + axisY*axisY + axisZ*axisZ)
	if length == 0 || math.IsNaN(length) || math.IsInf(length, 0) {
		return [3]float32{}
	}

	return [3]float32{float32(axisX / length), float32(axisY / length), float32(axisZ / length)}
}
