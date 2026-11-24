package render

import (
	"math"

	"github.com/karty-game/ashura/internal/meshdata"
)

func (draw DrawCall) localBounds() meshdata.Bounds {
	if draw.Prepared != nil {
		return draw.Prepared.Bounds()
	}

	return draw.Bounds
}

func transformPoint(matrix [16]float32, point [3]float32) [3]float32 {
	var result [3]float32
	for axis := range result {
		result[axis] = matrix[axis]*point[0] + matrix[axis+4]*point[1] + matrix[axis+8]*point[2] + matrix[axis+12]
	}

	return result
}

func midpoint(bounds meshdata.Bounds) [3]float32 {
	const half = 0.5

	var result [3]float32
	for axis := range result {
		result[axis] = bounds.Minimum[axis]*half + bounds.Maximum[axis]*half
	}

	return result
}

func absolute(value float32) float32 { return float32(math.Abs(float64(value))) }

const allShadowFaces = 1<<PointShadowFaces - 1
const maxShadowCullInstances = 64

// PointShadowFaceMask includes every cube face a caster might touch. Unknown
// geometry and non-finite or non-affine transforms take the conservative path.
func PointShadowFaceMask(draw DrawCall, light Light) uint8 {
	local := draw.localBounds()
	if !local.Valid || draw.Billboard || len(draw.Instances) > maxShadowCullInstances {
		return allShadowFaces
	}

	center := midpoint(local)

	var extent [3]float32
	for axis := range extent {
		extent[axis] = max(absolute(local.Maximum[axis]-center[axis]), absolute(center[axis]-local.Minimum[axis]))
	}

	var mask uint8

	for index := range max(1, len(draw.Instances)) {
		model := instanceAt(draw, index).Model

		mask |= pointShadowInstanceMask(model, center, extent, light)
		if mask == allShadowFaces {
			break
		}
	}

	return mask
}

func pointShadowInstanceMask(model [16]float32, center, extent [3]float32, light Light) uint8 {
	if model[3] != 0 || model[7] != 0 || model[11] != 0 || model[15] != 1 {
		return allShadowFaces
	}

	origin := transformPoint(model, center)

	var (
		low, high       [3]float32
		distanceSquared float64
	)

	for axis := range low {
		radius := absolute(model[axis])*extent[0] + absolute(model[axis+4])*extent[1] + absolute(model[axis+8])*extent[2]
		relative := origin[axis] - light.Direction[axis]
		// Protect the conservative classification at seams from f32 rounding.
		const roundingAllowance = 1e-5

		padding := (absolute(origin[axis]) + absolute(light.Direction[axis]) + radius + 1) * roundingAllowance

		low[axis], high[axis] = relative-radius-padding, relative+radius+padding
		if !pointFinite(low[axis]) || !pointFinite(high[axis]) {
			return allShadowFaces
		}

		nearest := float64(max(low[axis], -high[axis], 0))
		distanceSquared += nearest * nearest
	}

	lightRange := float64(light.GroundColor[0])
	if distanceSquared > lightRange*lightRange {
		return 0
	}

	var mask uint8

	for axis := range low {
		const facesPerAxis = 2

		face := axis * facesPerAxis
		sideA, sideB := (axis+1)%len(low), (axis+len(low)-1)%len(low)

		nearestSide := max(low[sideA], -high[sideA], low[sideB], -high[sideB], 0)
		if high[axis]*pointFaceGuard >= nearestSide {
			mask |= 1 << face
		}

		if -low[axis]*pointFaceGuard >= nearestSide {
			mask |= 1 << (face + 1)
		}
	}

	return mask
}
