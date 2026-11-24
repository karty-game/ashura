package engine

import (
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
)

type perspectiveFixture struct{}

func (perspectiveFixture) RuntimeProjection() (uint8, float32, float32, float32, float32) {
	return render.PerspectiveProjection, defaultFOV, 0, clipDefaultNear, clipDefaultFar
}

type orbitFixture struct {
	target           Vector3
	distance         float32
	minimum, maximum float32
}

func (controls *orbitFixture) RuntimeOrbitControls() (*Vector3, *float32, float32, float32, float32, float32, MouseButton) {
	return &controls.target, &controls.distance, controls.minimum, controls.maximum, 1, 1, MouseButtonLeft
}

type orthoFixture struct{ height float32 }

func (projection *orthoFixture) RuntimeProjection() (uint8, float32, float32, float32, float32) {
	return render.OrthographicProjection, 0, projection.height, clipDefaultNear, clipDefaultFar
}
func (projection *orthoFixture) RuntimeViewHeight() *float32 { return &projection.height }

type panFixture struct{}

func (panFixture) RuntimePanZoomControls() (float32, float32, float32, float32, MouseButton) {
	return 1, 1, 1, 10, MouseButtonRight
}

func TestOrbitMaintainsTargetAndClampsDistance(t *testing.T) {
	t.Parallel()

	const initialDistance, minimum, maximum = 5, 2, 8

	scene := NewWorld()
	view := scene.Add(
		DefaultTransform(),
		perspectiveFixture{},
		orbitFixture{target: Vector3{Y: 1}, distance: initialDistance, minimum: minimum, maximum: maximum},
	)
	inactive := scene.Add(DefaultTransform(), perspectiveFixture{}, orbitFixture{distance: initialDistance})
	input := platform.InputState{TouchDX: 30, TouchDY: 20, WheelY: 100}
	updateCameraControls(scene, time.Second, input)
	controls, _ := Get[orbitFixture](scene, view)
	transform := scene.Transform(view)

	if controls.distance != maximum {
		t.Fatalf("distance=%v, want %v", controls.distance, maximum)
	}

	assertOrbitTarget(t, transform, controls.target, controls.distance)

	if *scene.Transform(inactive) != DefaultTransform() {
		t.Fatal("inactive camera moved")
	}

	input.WheelY = -100
	updateCameraControls(scene, 0, input)

	if controls.distance != minimum {
		t.Fatalf("distance=%v, want %v", controls.distance, minimum)
	}

	assertOrbitTarget(t, transform, controls.target, controls.distance)
}

func assertOrbitTarget(t *testing.T, transform *Transform, target Vector3, distance float32) {
	t.Helper()
	// Check the actual rendering projection rather than repeating the orbit solver.
	projection := cameraViewProjection(*transform, projectionSettings{kind: render.PerspectiveProjection}, 1)
	clipX := projection[0]*target.X + projection[4]*target.Y + projection[8]*target.Z + projection[12]
	clipY := projection[1]*target.X + projection[5]*target.Y + projection[9]*target.Z + projection[13]
	clipW := projection[3]*target.X + projection[7]*target.Y + projection[11]*target.Z + projection[15]

	const tolerance = .0001
	if clipW <= 0 || math.Abs(float64(clipX/clipW)) > tolerance || math.Abs(float64(clipY/clipW)) > tolerance {
		t.Fatalf("orbit target projects off center: clip=(%v,%v,%v)", clipX, clipY, clipW)
	}

	offset := Vector3{X: transform.Position.X - target.X, Y: transform.Position.Y - target.Y, Z: transform.Position.Z - target.Z}

	actualDistance := float32(math.Sqrt(float64(offset.X*offset.X + offset.Y*offset.Y + offset.Z*offset.Z)))
	if math.Abs(float64(actualDistance-distance)) > tolerance {
		t.Fatalf("target distance=%v, want %v", actualDistance, distance)
	}
}

func TestOrthographicOrbitChangesExtentWithoutDolly(t *testing.T) {
	t.Parallel()

	const distance, height, minimum, maximum = 12, 4, 1, 8

	scene := NewWorld()
	view := scene.Add(
		DefaultTransform(),
		orthoFixture{height: height},
		orbitFixture{distance: distance, minimum: minimum, maximum: maximum},
	)
	updateCameraControls(scene, 0, platform.InputState{})
	initial := *scene.Transform(view)

	controls, _ := Get[orbitFixture](scene, view)
	if controls.distance != distance {
		t.Fatalf("orthographic extent bounds changed orbit distance from %v to %v", distance, controls.distance)
	}

	updateCameraControls(scene, 0, platform.InputState{Pinch: -float32(math.Log(2))})

	projection, _ := Get[orthoFixture](scene, view)
	if projection.height != height/2 {
		t.Fatalf("view height=%v, want %v", projection.height, height/2)
	}

	if *scene.Transform(view) != initial {
		t.Fatal("orthographic zoom moved camera instead of changing extent")
	}
}

func TestPanZoomUsesLogicalPixelsAndViewPlane(t *testing.T) {
	t.Parallel()

	const height, logicalHeight, delta = 10, 100, 20

	scene := NewWorld()
	view := scene.Add(DefaultTransform(), orthoFixture{height: height}, panFixture{})
	// Hover deltas have no effect; a recorded right-button drag pans the view.
	input := platform.InputState{Height: logicalHeight * 2, LogicalHeight: logicalHeight, MouseDX: delta}
	updateCameraControls(scene, time.Second, input)

	if scene.Transform(view).Position != (Vector3{}) {
		t.Fatal("panning without a drag")
	}

	input.DragDX[MouseButtonRight] = delta
	input.DragDY[MouseButtonRight] = delta
	updateCameraControls(scene, time.Second, input)

	if got := scene.Transform(view).Position; got != (Vector3{X: -2, Y: 2}) {
		t.Fatalf("pan=%v, want (-2,2,0)", got)
	}

	input = platform.InputState{WheelY: -100}
	updateCameraControls(scene, 0, input)

	projection, _ := Get[orthoFixture](scene, view)
	if projection.height != 1 {
		t.Fatalf("view height passed lower bound: %v", projection.height)
	}
}

func TestFreeTouchIsIndependentOfFrameDuration(t *testing.T) {
	t.Parallel()

	input := platform.InputState{LogicalHeight: 100, TouchDX: 12, TouchDY: -8, TouchPanX: 10, TouchPanY: 5, Pinch: -.25}
	scene := NewWorld()
	first := scene.Add(DefaultTransform(), cameraFixture{})
	second := scene.Add(DefaultTransform(), cameraFixture{})
	updateCameraControls(scene, time.Second, input)

	if err := scene.SetActiveCamera(second); err != nil {
		t.Fatal(err)
	}

	updateCameraControls(scene, 0, input)

	if *scene.Transform(first) != *scene.Transform(second) {
		t.Fatal("accumulated touch input was scaled by frame time")
	}

	if scene.Transform(first).Position == (Vector3{}) || scene.Transform(first).Rotation == (Vector3{}) {
		t.Fatal("touch did not move and rotate free camera")
	}
}

func TestCameraWithoutControlsPreservesAuthoredPose(t *testing.T) {
	t.Parallel()

	scene := NewWorld()
	pose := DefaultTransform()
	pose.Position = Vector3{X: 1, Y: 2, Z: 3}
	pose.Rotation.X = -math.Pi / 2
	view := scene.Add(pose, perspectiveFixture{})
	updateCameraControls(scene, time.Second, platform.InputState{TouchDX: 100, Pinch: 1})

	if *scene.Transform(view) != pose {
		t.Fatal("camera without controls was changed")
	}

	if err := scene.Set(view, cameraFixture{}); err != nil {
		t.Fatal(err)
	}

	updateCameraControls(scene, time.Second, platform.InputState{})

	if *scene.Transform(view) != pose {
		t.Fatal("idle free controls changed authored pose")
	}
}

func TestReleasedDragIsConsumedOnce(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                 string
		projection, controls any
		button               MouseButton
	}{
		{name: "free", projection: perspectiveFixture{}, controls: cameraFixture{}, button: MouseButtonLeft},
		{name: "orbit", projection: perspectiveFixture{}, controls: orbitFixture{distance: 5}, button: MouseButtonLeft},
		{name: "pan", projection: orthoFixture{height: 5}, controls: panFixture{}, button: MouseButtonRight},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			scene := NewWorld()
			view := scene.Add(DefaultTransform(), fixture.projection, fixture.controls)
			updateCameraControls(scene, 0, platform.InputState{})
			before := *scene.Transform(view)
			input := platform.InputState{Height: 100}
			input.DragDX[fixture.button] = 20
			input.DragDY[fixture.button] = 10
			// The button was released after this motion, before the frame callback.
			updateCameraControls(scene, 0, input)

			after := *scene.Transform(view)
			if before == after {
				t.Fatal("completed drag was dropped")
			}

			updateCameraControls(scene, 0, platform.InputState{Height: 100})

			if *scene.Transform(view) != after {
				t.Fatal("completed drag was replayed in the next frame")
			}
		})
	}
}

func TestPointerLockDoesNotDoubleCountDrag(t *testing.T) {
	t.Parallel()

	scene := NewWorld()
	first := scene.Add(DefaultTransform(), cameraFixture{})
	second := scene.Add(DefaultTransform(), cameraFixture{})
	input := platform.InputState{PointerLocked: true, MouseDX: 20, MouseDY: 10}
	input.DragDX[MouseButtonLeft] = 20
	input.DragDY[MouseButtonLeft] = 10
	updateCameraControls(scene, 0, input)

	if err := scene.SetActiveCamera(second); err != nil {
		t.Fatal(err)
	}

	input.DragDX = [8]float32{}
	input.DragDY = [8]float32{}
	updateCameraControls(scene, 0, input)

	if *scene.Transform(first) != *scene.Transform(second) {
		t.Fatal("pointer lock consumed both relative motion and button drag")
	}
}

func TestPointerCaptureFollowsControlPrecedence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		extra any
		want  bool
	}{
		{name: "free", want: true},
		{name: "orbit overrides free", extra: orbitFixture{distance: 5}},
		{name: "pan overrides free", extra: panFixture{}},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			scene := NewWorld()
			scene.Add(DefaultTransform(), cameraFixture{}, fixture.extra)

			frame, err := makeFrame(scene, 100, 100)
			if err != nil {
				t.Fatal(err)
			}

			if frame.PointerLock != fixture.want {
				t.Fatalf("pointer capture=%v, want %v", frame.PointerLock, fixture.want)
			}
		})
	}
}
