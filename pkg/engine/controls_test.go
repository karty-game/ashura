package engine

import (
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
)

type cameraFixture struct{}

func (cameraFixture) RuntimeProjection() (uint8, float32, float32, float32, float32) {
	return render.PerspectiveProjection, defaultFOV, 0, clipDefaultNear, clipDefaultFar
}

func (cameraFixture) RuntimeFreeControls() (float32, float32) { return 1, 1 }

func TestFreeCameraMovement(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  uint8
		yaw  float32
		want Vector3
	}{
		{name: "forward", key: platform.KeyCodeW, want: Vector3Forward()},
		{name: "backward", key: platform.KeyCodeS, want: Vector3Backward()},
		{name: "left", key: platform.KeyCodeA, want: Vector3Left()},
		{name: "right", key: platform.KeyCodeD, want: Vector3Right()},
		{name: "up", key: platform.KeyCodeSpace, want: Vector3Up()},
		{name: "down", key: platform.KeyCodeShift, want: Vector3Down()},
		{name: "rotated forward", key: platform.KeyCodeW, yaw: -math.Pi / 2, want: Vector3Right()},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			scene := NewWorld()
			transform := DefaultTransform()
			transform.Rotation.Y = fixture.yaw
			view := scene.Add(transform, cameraFixture{})

			var input platform.InputState

			input.Keys[fixture.key] = true
			updateFreeCameraControls(scene, time.Second, input)
			got := scene.Transform(view).Position

			const tolerance = 0.00001
			if math.Abs(float64(got.X-fixture.want.X)) > tolerance || math.Abs(float64(got.Y-fixture.want.Y)) > tolerance ||
				math.Abs(float64(got.Z-fixture.want.Z)) > tolerance {
				t.Fatalf("position = %v, want %v", got, fixture.want)
			}
		})
	}
}

func TestCameraSelectionSkipsMissingTransform(t *testing.T) {
	t.Parallel()

	scene := NewWorld()
	scene.Add(cameraFixture{})
	view := scene.Add(DefaultTransform(), cameraFixture{})

	got, found := activeCameraEntity(scene)
	if !found || got != view {
		t.Fatal("controls did not select the first usable camera")
	}
}

func TestClipPlanesKeepFarBeyondNear(t *testing.T) {
	t.Parallel()

	near, far := clipPlanes(clipDefaultFar*2, clipDefaultFar)
	if far <= near {
		t.Fatalf("near %v, far %v: clipping range is reversed", near, far)
	}
}
