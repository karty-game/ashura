//go:build !js

package lightmapper

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"
)

func TestGPUAnalyticCountIncludesOnlyIndependentRaySurfaces(t *testing.T) {
	t.Parallel()

	source := Scene{
		Emitters: []Emitter{
			{Kind: Point},
			{Kind: Spot},
			{Kind: Rectangle},
			{Kind: Disk},
			{Kind: Sphere},
			{Kind: EmissiveTriangle},
			{Kind: Directional},
			{Kind: Sun},
			{Kind: EnvironmentSource},
		},
	}
	// Zero intensity must not remove a finite occluding analytic surface.
	words := gpuPackSceneHeader(make(gpuWords, gpuSceneHeaderWords), &PreparedScene{source: source})
	if words[1] != gpuSceneVersion || words[gpuAnalyticCountWord] != 3 {
		t.Fatal("wrong analytic geometry classification", words[1], words[gpuAnalyticCountWord])
	}
}

func TestGPUSceneVersionAndAnalyticCountAdmission(t *testing.T) {
	t.Parallel()
	prepared := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .2, .2}}})

	scene, err := PackGPUScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := PackGPUTile(prepared, []Receiver{gpuFixtureReceiver(1, 0)}, Options{Outputs: OutputRGB, Samples: 2})
	if err != nil {
		t.Fatal(err)
	}

	for _, change := range []func([]byte){
		func(data []byte) { binary.LittleEndian.PutUint32(data[4:], gpuSceneVersion-1) },
		func(data []byte) {
			binary.LittleEndian.PutUint32(data[gpuAnalyticCountWord*gpuWordBytes:], binary.LittleEndian.Uint32(data[gpuEmitterCountWord*gpuWordBytes:])+1)
		},
	} {
		changed := scene
		changed.Data = append([]byte(nil), scene.Data...)
		change(changed.Data)

		if err = gpuValidatePackets(changed, tile); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid scene packet admitted", err)
		}
	}
}

//nolint:paralleltest // This hardware regression owns one native GPU worker.
func TestHardwareGPUAnalyticSurfaceKinds(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{MemoryBytes: 128 << 20})
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := worker.Close(); err != nil {
			t.Error(err)
		}
	}()

	for _, kind := range []EmitterKind{Rectangle, Disk, Sphere} {
		finite := Emitter{
			ID:        1,
			Kind:      kind,
			Position:  Vec3{0, 0, 2},
			Direction: Vec3{0, 0, -1},
			Tangent:   Vec3{1, 0, 0},
			Intensity: Vec3{1, 1, 1},
		}
		if kind == Rectangle {
			finite.Size = Vec2{1, 1}
		} else {
			finite.Radius = .5
		}

		source := Scene{
			Emitters:    []Emitter{finite, {ID: 2, Kind: Point, Position: Vec3{3, 0, 2}, Intensity: Vec3{.1, .1, .1}}},
			Environment: Environment{Sky: Vec3{.1, .1, .1}},
		}
		scene := gpuFixtureScene(t, source)
		options := Options{Outputs: OutputRGB | OutputRNM, Samples: 256, Seed: 29}
		receivers := []Receiver{testPointReceiver()}

		actual, bakeErr := worker.Bake(ctx, scene, receivers, options)
		if bakeErr != nil {
			t.Fatal(kind, bakeErr)
		}

		expected, referenceErr := BakeReference(ctx, scene, receivers, options)
		if referenceErr != nil {
			t.Fatal(referenceErr)
		}

		gpuCompare(t, actual, expected, .003)
	}
}
