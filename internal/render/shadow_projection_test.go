package render

import (
	"math"
	"slices"
	"testing"
)

func TestCameraPerspectiveUpdatesEveryLocalShadow(t *testing.T) {
	t.Parallel()

	frame := shadowCameraFrame()
	frame.Draws = []DrawCall{{
		ID: 1, Vertices: []Vertex{{}}, Indices: []uint32{0, 0, 0},
		Transform: [16]float32{0: 1, 5: 1, 10: 1, 14: -4, 15: 1},
	}}
	point := Light{Kind: PointLightKind, CastShadows: true, Intensity: 1, Direction: [3]float32{0, 0, -2},
		GroundColor: [3]float32{8}, ShadowResolution: 512}
	spot := Light{Kind: SpotLightKind, CastShadows: true, Intensity: 1, SpotDirection: [3]float32{0, 0, -1},
		GroundColor: [3]float32{8}, OuterAngle: .7}
	second := spot
	second.Direction[0] = 2
	frame.Lights = []Light{point, spot, second}

	var (
		pointState PointShadowState
		spotState  SpotShadowState
	)

	spotDirty, spotMasks := spotState.Update(frame)
	if spotDirty != 3 || pointState.Update(frame, point) == 0 {
		t.Fatal("initial camera-aware depth is missing")
	}

	if pointState.views[5] == pointState.fixed[5] || spotState.Views[0].Matrix == spotState.fixed[0] ||
		spotState.Views[1].Matrix == spotState.fixed[1] {
		t.Fatal("camera perspective was not applied to every local light")
	}

	pointView := pointState.views[5]
	pointMasks := slices.Clone(pointState.FaceMasks())
	oldSpotMasks := slices.Clone(spotMasks)

	// Simulate a lateral camera translation with the casters and lights fixed.
	frame.ViewProjection[12] += 1.5

	spotDirty, spotMasks = spotState.Update(frame)
	if spotDirty != 3 || pointState.Update(frame, point) == 0 || pointView == pointState.views[5] {
		t.Fatal("camera-only motion left a stale light view")
	}

	if !slices.Equal(oldSpotMasks, spotMasks) || !slices.Equal(pointMasks, pointState.FaceMasks()) {
		t.Fatal("camera-only motion changed the physical caster sets")
	}

	spotDirty, _ = spotState.Update(frame)
	if spotDirty != 0 || pointState.Update(frame, point) != 0 {
		t.Fatal("stationary perspective camera regenerated depth")
	}
}

func TestWarpedShadowRetainsOffscreenOccluderOnReceiverRay(t *testing.T) {
	t.Parallel()

	camera := cameraForShadows(shadowCameraFrame())
	position := [3]float32{4, 2, 0}
	fixed := spotMatrix(spotPose{position: position, direction: [3]float32{0, 0, -1}, radius: 12, angle: .6})

	matrix := shadowMatrixRows(shadowProjection(camera, position, fixed))
	if matrix == shadowMatrixRows(fixed) {
		t.Fatal("expected a post-perspective projection")
	}

	caster := shadowVec{3.2, 1.6, -2}
	if shadowNDC(shadowMatrixRows(camera.matrix), caster)[0] <= 1 {
		t.Fatal("fixture caster must be outside the camera frustum")
	}

	occluder := shadowNDC(matrix, caster)

	receiver := shadowNDC(matrix, shadowVec{0, 0, -10})
	if math.Abs(occluder[0]-receiver[0]) > 1e-5 || math.Abs(occluder[1]-receiver[1]) > 1e-5 ||
		occluder[2] < 0 || occluder[2] >= receiver[2] || receiver[2] > 1 {
		t.Fatalf("occluder = %v, receiver = %v", occluder, receiver)
	}
}

func shadowCameraFrame() Frame {
	const near, far = float32(.1), float32(30)

	return Frame{
		CameraNear: near, CameraFar: far, PerspectiveCamera: true, ShadowCasterRevision: 1,
		ViewProjection: [16]float32{0: 1.3, 5: 1.7, 10: far / (near - far), 11: -1, 14: near * far / (near - far)},
	}
}

func shadowNDC(rows shadowRows, point shadowVec) shadowVec {
	w := shadowEvaluate(rows[3], point)

	return shadowVec{shadowEvaluate(rows[0], point) / w, shadowEvaluate(rows[1], point) / w, shadowEvaluate(rows[2], point) / w}
}
