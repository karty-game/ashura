package engine

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestDecalRetainedOrderAndLifetime(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	image := testDecalTexture(t, app)
	value := DecalProjector{
		Image: DecalImage{Texture: image}, Normal: Vector3Up(), Size: Vector3One(),
		Tint: ColorWhite(), ReceiverMask: 1 << 31,
	}
	world := NewWorld()
	app.scene = world
	value.Position.X, value.Order = 1, 4
	first := world.Add(value)
	value.Position.X, value.Order = 2, 0
	second := world.Add(value)
	value.Position.X, value.Order = 3, 4
	third := world.Add(value)

	snapshot := func() *worldDecals {
		t.Helper()

		if err := world.decals.snapshot(); err != nil {
			t.Fatal(err)
		}

		return world.decals
	}
	assertOrder := func(want ...float32) {
		t.Helper()

		state := snapshot()
		if state.frame.Count != len(want) {
			t.Fatalf("active count %d, want %d", state.frame.Count, len(want))
		}

		for index, expected := range want {
			record := state.frame.Records[index]

			position := math.Float32frombits(binary.LittleEndian.Uint32(record[12:]))
			if position != expected || binary.LittleEndian.Uint32(record[80:]) != value.ReceiverMask {
				t.Fatalf("unexpected record %d", index)
			}
		}
	}
	assertOrder(2, 1, 3)

	revision := snapshot().frame.Revision

	assertOrder(2, 1, 3)

	if snapshot().frame.Revision != revision {
		t.Fatal("unchanged projectors were rebuilt")
	}

	if err := app.UnloadTexture(image); !errors.Is(err, ErrInUse) {
		t.Fatal("live decal texture should block unload", err)
	}

	instance, _ := Get[DecalProjector](world, third)
	instance.Order = -1

	assertOrder(3, 2, 1)

	RemoveComponent[DecalProjector](world, first)
	world.Remove(second)

	instance.Tint.A = 0

	assertOrder()

	if err := app.UnloadTexture(image); err != nil {
		t.Fatal("disabled and removed projectors should release the texture", err)
	}
}

func TestDecalAdmissionAndFeature(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	image, other := testDecalTexture(t, app), testDecalTexture(t, app)
	world := NewWorld()
	value := DecalProjector{
		Image: DecalImage{Texture: image}, Normal: Vector3Up(), Size: Vector3One(),
		Tint: ColorWhite(), ReceiverMask: 1,
	}
	world.Add(value)

	frame, err := makeFrame(world, 16, 9)
	if err != nil {
		t.Fatal(err)
	}

	if err := app.validateFrameResources(frame); !errors.Is(err, ErrFeatureMissing) {
		t.Fatal("projector should require explicit feature registration", err)
	}

	app.featureMask = 1 << (platform.DecalFeature - 1)
	if err := app.validateFrameResources(frame); err != nil {
		t.Fatal(err)
	}

	value.Image.Texture = other

	otherEntity := world.Add(value)
	if _, err := makeFrame(world, 16, 9); !errors.Is(err, errDecalAtlas) {
		t.Fatal("two atlases should fail admission", err)
	}

	world.Remove(otherEntity)

	value.Image.Texture, value.Size.Z = image, 0

	invalid := world.Add(value)
	if _, err := makeFrame(world, 16, 9); !errors.Is(err, errDecalProjection) {
		t.Fatal("degenerate projection should fail admission", err)
	}

	world.Remove(invalid)

	value.Size = Vector3One()

	for range MaxDecalProjectors {
		world.Add(value)
	}

	if _, err := makeFrame(world, 16, 9); !errors.Is(err, errDecalLimit) {
		t.Fatal("projector limit should be enforced", err)
	}
}

func testDecalTexture(t *testing.T, app *App) *texture.Texture {
	t.Helper()

	data := texture.Data{Width: 1, Height: 1, ColorSpace: texture.SRGB, Levels: [][]byte{{255, 0, 0, 128}}}

	image, err := app.NewTexture(data)
	if err != nil {
		t.Fatal(err)
	}

	return image
}
