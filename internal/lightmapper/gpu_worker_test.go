//go:build !js

package lightmapper

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"testing"
	"time"
)

func gpuFixtureReceiver(id uint64, positionX float32) Receiver {
	return Receiver{
		ID:              id,
		Side:            1,
		Coverage:        .5,
		Position:        Vec3{positionX, 0, 0},
		GeometricNormal: Vec3{0, 1, 0},
		Normal:          Vec3{0, 1, 0},
		Tangent:         Vec3{1, 0, 0},
		Handedness:      1,
		Patches: []ReceiverPatch{
			{
				Positions: [3]Vec3{{positionX - .01, 0, -.01}, {positionX - .01, 0, .01}, {positionX + .01, 0, -.01}},
				Normals:   [3]Vec3{{0, 1, 0}, {0, 1, 0}, {0, 1, 0}},
				UV:        [3]Vec2{{0, 0}, {0, 1}, {1, 0}},
				Area:      .5,
			},
		},
	}
}
func gpuFixtureScene(t *testing.T, source Scene) *PreparedScene {
	t.Helper()

	source.ID = "gpu-fixture"
	source.Space = "test"
	source.MetresPerUnit = 1

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	return prepared
}

func TestGPUProtocolAdmissionAccountsReadbackAndPathStorage(t *testing.T) {
	t.Parallel()
	prepared := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .2, .2}, Ground: Vec3{.2, .2, .2}}})

	scene, err := PackGPUScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := PackGPUTile(prepared, []Receiver{gpuFixtureReceiver(1, 0)}, Options{Outputs: OutputRGB | OutputRNM, Samples: 4})
	if err != nil {
		t.Fatal(err)
	}

	expected := uint64(64 + 12 + gpuPathHeaderBytes + len(scene.Data) + len(tile.Data) + (gpuPathWords+2)*4 + gpuResultWords*4*2)

	worker := &GPUWorker{
		options: GPUWorkerOptions{MemoryBytes: expected - 1},
		device:  GPUDevice{MaxBufferSize: 256 << 20, MaxStorageBufferBindingSize: 128 << 20, MaxStorageBuffersPerShaderStage: 4},
	}
	if err = worker.admit(scene, tile); !errors.Is(err, ErrBounds) {
		t.Fatal("unaccounted allocation", err)
	}

	worker.options.MemoryBytes = expected
	if err = worker.admit(scene, tile); err != nil {
		t.Fatal(err)
	}

	tile.Options.Bounces = 33
	if err = worker.admit(scene, tile); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbounded bounce job", err)
	}
}

func TestGPUScenePacketContainsTwoLevelEnvironmentDistribution(t *testing.T) {
	t.Parallel()
	prepared := gpuFixtureScene(
		t,
		Scene{
			Textures:    []Texture{{Width: 2, Height: 2, Pixels: []Vec3{{1e20, 1e20, 1e20}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}}},
			Environment: Environment{Texture: 1},
		},
	)

	packet, err := PackGPUScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	word := func(i int) uint32 { return binary.LittleEndian.Uint32(packet.Data[i*4:]) }
	if word(30) != 3 || word(32) != 6 {
		t.Fatal("environment distribution missing", word(30), word(32))
	}
}

//nolint:paralleltest // This single hardware worker validates resident reuse and ordered scene transitions.
func TestHardwareGPUProductionLightmapper(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	qualifyNativeProductionLightmapper(t, false)
}

//nolint:paralleltest // Explicit software-CI qualification owns one native worker; it is not hardware graphics/performance evidence.
func TestSoftwareCIProductionLightmapper(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_SOFTWARE_CI_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_SOFTWARE_CI_TEST=1 for explicit native Vulkan CPU functional qualification")
	}

	qualifyNativeProductionLightmapper(t, true)
}

func qualifyNativeProductionLightmapper(t *testing.T, softwareForCI bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{
		MemoryBytes: 128 << 20, SoftwareForCI: softwareForCI,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })

	if worker.Device().SoftwareForCI != softwareForCI {
		t.Fatal("native software mode acknowledgement mismatch", worker.Device())
	}

	t.Log("native adapter", worker.Device().Adapter)

	receivers := []Receiver{gpuFixtureReceiver(101, 0), gpuFixtureReceiver(102, 1)}
	gpuEnvironmentQualification(t, ctx, worker, receivers)
	gpuManyLightQualification(t, ctx, worker, receivers[:1])
	gpuAreaQualification(t, ctx, worker, receivers[:1])
	gpuQualification(t, ctx, worker)
}

func gpuCompare(t *testing.T, actual, expected Result, tolerance float64) {
	t.Helper()

	if len(actual.Estimates) != len(expected.Estimates) {
		t.Fatal("result cardinality")
	}

	for i := range actual.Estimates {
		for k := range 4 {
			for c := range 3 {
				positionX, y := actual.Estimates[i].Statistics[k][c], expected.Estimates[i].Statistics[k][c]
				if math.Abs(float64(positionX-y)) > tolerance {
					t.Fatalf("receiver%d kernel%d channel%d GPU%g reference%g", i, k, c, positionX, y)
				}
			}
		}
	}
}

func gpuEnvironmentQualification(t *testing.T, ctx context.Context, worker *GPUWorker, receivers []Receiver) {
	t.Helper()
	scene := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .15, .1}, Ground: Vec3{.2, .15, .1}}})
	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 256, Seed: 29}

	result, err := worker.Bake(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	reference, err := BakeReference(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	gpuCompare(t, result, reference, .003)

	for _, e := range result.Estimates {
		for c := range 3 {
			flat := (e.RNM[0][c] + e.RNM[1][c] + e.RNM[2][c]) / 3
			if math.Abs(float64(flat-e.Statistics[0][c])) > 1e-5 {
				t.Fatal("GPU projection lost flat energy", flat, e.Statistics[0])
			}
		}
	}

	if _, err = worker.Bake(ctx, scene, receivers[:1], options); err != nil {
		t.Fatal(err)
	}

	if worker.SceneUploads() != 1 {
		t.Fatal("unchanged scene uploaded again", worker.SceneUploads())
	}

	probes := [][]Vec3{{{0, 0, 1}, {.8660254, 0, .5}}, {{0, 0, 1}, {0, .8660254, .5}}}
	independent := options
	independent.Seed = 713

	responses, err := worker.BakeResponses(ctx, scene, receivers, independent, probes)
	if err != nil {
		t.Fatal(err)
	}

	for i, e := range responses {
		for probe, n := range probes[i] {
			for c, env := range []float32{.2, .15, .1} {
				expected := env * (1 + n[2]) * .5
				if math.Abs(float64(e.Means[probe][c]-expected)) > .004 {
					t.Fatal("full response differs from analytic hemispherical environment", i, probe, e.Means[probe], expected)
				}
			}
		}
	}

	gpuReconstructionQualification(t, ctx, worker, scene, receivers, result)
}

func gpuManyLightQualification(t *testing.T, ctx context.Context, worker *GPUWorker, receivers []Receiver) {
	t.Helper()

	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 32, Seed: 29}
	single := gpuFixtureScene(t, Scene{Emitters: []Emitter{{ID: 1, Kind: Point, Position: Vec3{0, 2, 0}, Intensity: Vec3{16, 12, 8}}}})
	options.Samples = 32

	one, err := worker.Bake(ctx, single, receivers[:1], options)
	if err != nil {
		t.Fatal(err)
	}

	many := single.Source()
	many.ID = "many-light-fixture"

	many.Emitters = make([]Emitter, 16384)
	for i := range many.Emitters {
		many.Emitters[i] = Emitter{
			ID:        uint64(i + 1),
			Kind:      Point,
			Position:  Vec3{0, 2, 0},
			Intensity: Vec3{16.0 / 16384, 12.0 / 16384, 8.0 / 16384},
		}
	}

	crowded := gpuFixtureScene(t, many)

	scaled, err := worker.Bake(ctx, crowded, receivers[:1], options)
	if err != nil {
		t.Fatal(err)
	}

	gpuCompare(t, scaled, one, .0001)
	t.Log("16384 overlapping lights preserve total energy")
}

func gpuAreaQualification(t *testing.T, ctx context.Context, worker *GPUWorker, receivers []Receiver) {
	t.Helper()

	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 512, Seed: 29}
	area := gpuFixtureScene(
		t,
		Scene{
			Emitters: []Emitter{
				{
					ID:        1,
					Kind:      Rectangle,
					Position:  Vec3{0, 2, 0},
					Direction: Vec3{0, -1, 0},
					Tangent:   Vec3{1, 0, 0},
					Size:      Vec2{1, 1},
					Intensity: Vec3{2, 1, .5},
				},
			},
		},
	)
	options.Samples = 512

	gpuArea, err := worker.Bake(ctx, area, receivers[:1], options)
	if err != nil {
		t.Fatal(err)
	}

	cpuArea, err := BakeReference(ctx, area, receivers[:1], options)
	if err != nil {
		t.Fatal(err)
	}

	gpuCompare(t, gpuArea, cpuArea, .002)
	t.Log("finite area MIS matches independent CPU transport")
}

func gpuReconstructionQualification(
	t *testing.T,
	ctx context.Context,
	worker *GPUWorker,
	scene *PreparedScene,
	receivers []Receiver,
	result Result,
) {
	t.Helper()

	reconstruction := ReconstructionOptions{
		Radius:         2,
		PlaneTolerance: .01,
		NormalCos:      .99,
		TangentCos:     .99,
		NoiseScale:     16,
		Strength:       1,
		MaximumHalo:    8,
	}
	edges := []ReconstructionEdge{{From: 101, To: 102, Weight: 1, Visible: true}, {From: 102, To: 101, Weight: 1, Visible: true}}

	filtered, err := worker.Reconstruct(ctx, scene, receivers, result.Estimates, edges, reconstruction)
	if err != nil {
		t.Fatal(err)
	}

	halo, err := ReconstructionSamples(scene.Source().Space, receivers, result.Estimates)
	if err != nil {
		t.Fatal(err)
	}

	expectedFiltered, err := ReconstructLinear([]uint64{101, 102}, halo, edges, reconstruction)
	if err != nil {
		t.Fatal(err)
	}

	for i, e := range filtered {
		for k := range 4 {
			for c := range 3 {
				if math.Abs(float64(e.Statistics[k][c]-expectedFiltered[i][k][c])) > 1e-5 {
					t.Fatal("compute reconstruction differs from reference", i, k, c)
				}
			}
		}
	}

	t.Log("independent full directional responses and compute reconstruction match analytic/reference results")
}
