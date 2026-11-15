package lightmapper

import (
	"errors"
	"testing"
)

func TestUnsupportedEmitterExtentCannotSilentlyBecomeDeltaLight(t *testing.T) {
	t.Parallel()

	cases := []Emitter{
		{Kind: Point, Radius: .1},
		{Kind: Spot, Radius: .1},
		{Kind: Rectangle, Radius: .1, Size: Vec2{1, 1}},
		{Kind: Disk, Radius: 1, Size: Vec2{1, 1}},
		{Kind: Sphere, Radius: 1, Size: Vec2{1, 1}},
		{Kind: EmissiveTriangle, Radius: 1},
		{Kind: Directional, Radius: .01},
		{Kind: Sun, Radius: .01, Range: 10},
		{Kind: EnvironmentSource, Size: Vec2{1, 1}},
	}
	for _, emitter := range cases {
		emitter.ID = 1
		emitter.Direction = Vec3{0, 0, -1}
		emitter.Intensity = Vec3{1, 1, 1}
		source := testCoreScene()
		source.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}}}
		source.Triangles = []Triangle{{ID: 2, Positions: [3]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, Material: 0}}

		source.Emitters = []Emitter{emitter}
		if _, err := PrepareScene(source); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unsupported source fields were silently accepted", emitter.Kind, err)
		}
	}
}

func TestSupportedFiniteSurfaceAndInfiniteSourceFieldsRemainAdmitted(t *testing.T) {
	t.Parallel()

	cases := []Emitter{
		{Kind: Point, Range: 10},
		{Kind: Spot, Range: 10, InnerCos: .9, OuterCos: .8},
		{Kind: Rectangle, Size: Vec2{1, 1}, Range: 10},
		{Kind: Disk, Radius: 1},
		{Kind: Sphere, Radius: 1},
		{Kind: Directional},
		{Kind: Sun, Radius: .01},
	}
	for _, emitter := range cases {
		emitter.ID = 1
		emitter.Position = Vec3{0, 0, 2}
		emitter.Direction = Vec3{0, 0, -1}
		emitter.Intensity = Vec3{1, 1, 1}
		source := testCoreScene()

		source.Emitters = []Emitter{emitter}
		if _, err := PrepareScene(source); err != nil {
			t.Fatal("supported source fields rejected", emitter.Kind, err)
		}
	}
}
