package render

import "math"

// The fit intersects two six-plane frusta. Enumerating plane triplets keeps
// the CPU work bounded independently of the number of draws in the scene.
const (
	shadowPlaneCount    = 12
	shadowPlaneTriplets = 220
	shadowFitEpsilon    = 1e-7
)

type shadowVec [3]float64
type shadowPlane [4]float64
type shadowRows [4]shadowPlane

type shadowCamera struct {
	matrix [16]float32
	near   float32
}

type shadowWarp struct {
	warp  shadowRows
	light shadowVec
}

type shadowFit struct {
	right, up, forward     shadowVec
	minX, maxX, minY, maxY float64
	near, far              float64
}

func cameraForShadows(frame Frame) shadowCamera {
	if !frame.PerspectiveCamera || frame.CameraNear <= 0 || !pointFinite(frame.CameraNear) {
		return shadowCamera{}
	}

	return shadowCamera{matrix: frame.ViewProjection, near: frame.CameraNear}
}

// shadowProjection fits a light view in the viewing camera's post-perspective
// space and composes the two transforms. The input view has its origin at the light.
func shadowProjection(camera shadowCamera, position [3]float32, fixed [16]float32) [16]float32 {
	if camera.near == 0 {
		return fixed
	}

	cameraRows := shadowMatrixRows(camera.matrix)
	lightRows := shadowMatrixRows(fixed)

	var storage [shadowPlaneTriplets]shadowVec

	receivers := shadowReceivers(cameraRows, lightRows, storage[:0])
	if len(receivers) == 0 {
		return fixed
	}

	light := shadowVec{float64(position[0]), float64(position[1]), float64(position[2])}
	winding := shadowDot(shadowCross(shadowNormal(lightRows[0]), shadowNormal(lightRows[1])), shadowNormal(lightRows[3]))
	post := warpIntoCamera(cameraRows, float64(camera.near), light, receivers)

	fit, fitted := fitWarpedShadow(post, light, lightRows, receivers, winding < 0)
	if !fitted {
		return fixed
	}

	matrix, finite := composeShadowMatrix(post, fit)
	if !finite {
		return fixed
	}

	return matrix
}

func shadowReceivers(camera, light shadowRows, dst []shadowVec) []shadowVec {
	var planes [shadowPlaneCount]shadowPlane

	for index, rows := range [2]shadowRows{camera, light} {
		frustum := [6]shadowPlane{
			shadowPlaneAdd(rows[3], rows[0], 1), shadowPlaneAdd(rows[3], rows[0], -1),
			shadowPlaneAdd(rows[3], rows[1], 1), shadowPlaneAdd(rows[3], rows[1], -1),
			rows[2], shadowPlaneAdd(rows[3], rows[2], -1),
		}
		for face, plane := range frustum {
			length := shadowLength(shadowVec{plane[0], plane[1], plane[2]})
			if length <= shadowFitEpsilon || math.IsNaN(length) {
				return dst
			}

			for axis := range plane {
				planes[index*len(frustum)+face][axis] = plane[axis] / length
			}
		}
	}

	for first := range len(planes) - 2 {
		for second := first + 1; second < len(planes)-1; second++ {
			for third := second + 1; third < len(planes); third++ {
				point, ok := intersectShadowPlanes(planes[first], planes[second], planes[third])
				if ok && insideShadowFrusta(&planes, point) {
					dst = append(dst, point)
				}
			}
		}
	}

	return dst
}

func intersectShadowPlanes(first, second, third shadowPlane) (shadowVec, bool) {
	normalA, normalB, normalC := shadowNormal(first), shadowNormal(second), shadowNormal(third)
	crossBC := shadowCross(normalB, normalC)

	determinant := shadowDot(normalA, crossBC)
	if math.Abs(determinant) <= shadowFitEpsilon {
		return shadowVec{}, false
	}

	crossCA, crossAB := shadowCross(normalC, normalA), shadowCross(normalA, normalB)

	var point shadowVec

	for axis := range point {
		point[axis] = -(first[3]*crossBC[axis] + second[3]*crossCA[axis] + third[3]*crossAB[axis]) / determinant
	}

	return point, true
}

func insideShadowFrusta(planes *[shadowPlaneCount]shadowPlane, point shadowVec) bool {
	padding := shadowFitEpsilon * (1 + shadowLength(point))

	for _, plane := range planes {
		if shadowEvaluate(plane, point) < -padding {
			return false
		}
	}

	return true
}

func warpIntoCamera(rows shadowRows, near float64, light shadowVec, receivers []shadowVec) shadowWarp {
	first, last := math.Inf(1), math.Inf(-1)

	for _, receiver := range receivers {
		value := shadowEvaluate(rows[3], receiver)
		first, last = min(first, value), max(last, value)
	}

	// The convex hull between the light and the receivers must stay in front
	// of the virtual camera plane. A small slide also avoids extreme warps
	// when the beam just touches the real camera's near plane.
	const minimumSlideFraction = .12

	slide := max(minimumSlideFraction*(last-first), near-min(first, shadowEvaluate(rows[3], light)))
	rows[3][3] += slide

	// This is the virtual camera's projected depth up to an affine change
	// of coordinates. Choose its scale from the depth of the fitted volume.
	for axis := range rows[2] {
		rows[2][axis] = -rows[3][axis]
	}

	rows[2][3] += max(first+slide, near)

	return shadowWarp{
		warp:  rows,
		light: applyShadowWarp(rows, light),
	}
}

func fitWarpedShadow(post shadowWarp, light shadowVec, lightView shadowRows, receivers []shadowVec, flipUp bool) (shadowFit, bool) {
	fit := shadowFit{
		minX: math.Inf(1), maxX: math.Inf(-1), minY: math.Inf(1), maxY: math.Inf(-1),
		near: math.Inf(1),
	}

	for _, receiver := range receivers {
		ray := shadowSubtract(applyShadowWarp(post.warp, receiver), post.light)

		unit := shadowUnit(ray)
		for axis := range fit.forward {
			fit.forward[axis] += unit[axis]
		}
	}

	fit.forward = shadowUnit(fit.forward)
	// Keep the face's horizontal orientation as the camera moves. The
	// derivative of the warp maps this tangent without crossing its pole.
	physicalRight := shadowUnit(shadowNormal(lightView[0]))
	warpSlope := shadowDot(shadowNormal(post.warp[3]), physicalRight)

	rayRight := shadowVec{}
	for row := range rayRight {
		rayRight[row] = shadowDot(shadowNormal(post.warp[row]), physicalRight) - post.light[row]*warpSlope
	}

	fit.right = shadowUnit(shadowCross(fit.forward, shadowCross(rayRight, fit.forward)))
	if fit.right == (shadowVec{}) {
		return fit, false
	}

	fit.up = shadowCross(fit.right, fit.forward)
	if !flipUp {
		fit.up = shadowCross(fit.forward, fit.right)
	}

	for _, receiver := range receivers {
		ray := shadowSubtract(applyShadowWarp(post.warp, receiver), post.light)

		depth := shadowDot(fit.forward, ray)
		if depth <= shadowFitEpsilon {
			return fit, false
		}

		x, y := shadowDot(fit.right, ray)/depth, shadowDot(fit.up, ray)/depth
		fit.minX, fit.maxX = min(fit.minX, x), max(fit.maxX, x)
		fit.minY, fit.maxY = min(fit.minY, y), max(fit.maxY, y)
		fit.far = max(fit.far, depth)

		// Include the physical light view's near slice along every fitted ray,
		// including rays with occluders outside the viewing frustum.
		axialDepth := shadowEvaluate(lightView[3], receiver)
		nearPoint := shadowNearPoint(light, receiver, axialDepth)
		nearRay := shadowSubtract(applyShadowWarp(post.warp, nearPoint), post.light)
		fit.near = min(fit.near, shadowDot(fit.forward, nearRay))
	}

	return fit, fit.near > shadowFitEpsilon && fit.far >= fit.near
}

func shadowNearPoint(light, receiver shadowVec, axialDepth float64) shadowVec {
	// A conservative slice closer to the emitter than the fixed light view near
	// plane also leaves room for the existing receiver bias.
	const depth = .01

	step := min(depth/axialDepth, 1)
	point := light

	for axis := range point {
		point[axis] += step * (receiver[axis] - light[axis])
	}

	return point
}

func composeShadowMatrix(post shadowWarp, fit shadowFit) ([16]float32, bool) {
	var matrix [16]float32

	width, height := fit.maxX-fit.minX, fit.maxY-fit.minY
	if width <= shadowFitEpsilon || height <= shadowFitEpsilon {
		return matrix, false
	}

	const lateralPadding, nearPadding, farPadding, midpoint = 1.04, .8, 1.02, .5

	near, far := fit.near*nearPadding, fit.far*farPadding
	depthScale := far / (far - near)
	centerX, centerY := fit.minX+width*midpoint, fit.minY+height*midpoint
	lightX, lightY := shadowDot(fit.right, post.light), shadowDot(fit.up, post.light)
	lightZ := shadowDot(fit.forward, post.light)

	for col := range 4 {
		warpColumn := shadowVec{post.warp[0][col], post.warp[1][col], post.warp[2][col]}
		warpW := post.warp[3][col]
		lightW := shadowDot(fit.forward, warpColumn) - lightZ*warpW
		x := shadowDot(fit.right, warpColumn) - lightX*warpW
		y := shadowDot(fit.up, warpColumn) - lightY*warpW
		matrix[col*4] = float32((x - centerX*lightW) * 2 / (width * lateralPadding))
		matrix[col*4+1] = float32((y - centerY*lightW) * 2 / (height * lateralPadding))
		matrix[col*4+2] = float32((lightW - near*warpW) * depthScale)
		matrix[col*4+3] = float32(lightW)
	}

	for _, value := range matrix {
		if !pointFinite(value) {
			return matrix, false
		}
	}

	return matrix, true
}

func shadowMatrixRows(matrix [16]float32) shadowRows {
	var rows shadowRows
	for row := range rows {
		for col := range rows[row] {
			rows[row][col] = float64(matrix[col*4+row])
		}
	}

	return rows
}

func shadowPlaneAdd(a, b shadowPlane, sign float64) shadowPlane {
	return shadowPlane{a[0] + sign*b[0], a[1] + sign*b[1], a[2] + sign*b[2], a[3] + sign*b[3]}
}

func shadowNormal(plane shadowPlane) shadowVec {
	return shadowVec{plane[0], plane[1], plane[2]}
}

func shadowEvaluate(plane shadowPlane, point shadowVec) float64 {
	return shadowDot(shadowNormal(plane), point) + plane[3]
}

func applyShadowWarp(matrix shadowRows, point shadowVec) shadowVec {
	w := shadowEvaluate(matrix[3], point)

	return shadowVec{shadowEvaluate(matrix[0], point) / w, shadowEvaluate(matrix[1], point) / w, shadowEvaluate(matrix[2], point) / w}
}

func shadowSubtract(a, b shadowVec) shadowVec {
	return shadowVec{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func shadowDot(a, b shadowVec) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func shadowCross(a, b shadowVec) shadowVec {
	return shadowVec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func shadowLength(value shadowVec) float64 { return math.Sqrt(shadowDot(value, value)) }

func shadowUnit(value shadowVec) shadowVec {
	length := shadowLength(value)
	if length <= shadowFitEpsilon {
		return shadowVec{}
	}

	return shadowVec{value[0] / length, value[1] / length, value[2] / length}
}
