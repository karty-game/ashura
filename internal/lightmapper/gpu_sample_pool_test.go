//go:build !js

package lightmapper

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"
	"time"
)

//nolint:paralleltest // One hardware worker at a time checks deterministic sample tails across admitted pool widths.
func TestHardwareGPUSamplePoolPreservesOrderedTail(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	scene, receivers := samplePoolTailFixture(t)
	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 5, Bounces: 3, Seed: 29, RouletteDepth: 3}

	packed, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := PackGPUTile(scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	var reference []byte

	var referenceCounts GPUTraceCounts

	for _, profile := range []bool{false, true} {
		for _, lanes := range []uint64{1, 2, 5} {
			raw, counts := runSamplePoolTail(t, ctx, packed, tile, profile, lanes)
			if reference == nil {
				reference = bytes.Clone(raw)
			}

			assertSamplePoolTail(t, raw, reference, tile)

			if profile {
				if referenceCounts.Rays == 0 {
					referenceCounts = counts
				}

				if counts != referenceCounts || counts.Rays == 0 || counts.ShadowRays == 0 {
					t.Fatal("sample identity or tail changed continuation/shadow counts", lanes, counts, referenceCounts)
				}
			}
		}
	}
}

func samplePoolTailFixture(t *testing.T) (*PreparedScene, []Receiver) {
	t.Helper()

	scene := gpuFixtureScene(t, Scene{
		Materials: []Material{{Reflectance: Vec3{.6, .5, .4}, DoubleSided: true}},
		Triangles: []Triangle{
			{ID: 1, Positions: [3]Vec3{{-2, 2, -2}, {2, 2, -2}, {-2, 2, 2}}, Normals: [3]Vec3{{0, -1, 0}, {0, -1, 0}, {0, -1, 0}}},
			{ID: 2, Positions: [3]Vec3{{2, 2, -2}, {2, 2, 2}, {-2, 2, 2}}, Normals: [3]Vec3{{0, -1, 0}, {0, -1, 0}, {0, -1, 0}}},
		},
		Emitters:    []Emitter{{ID: 1, Kind: Point, Position: Vec3{0, 1, 0}, Intensity: Vec3{4, 3, 2}}},
		Environment: Environment{Sky: Vec3{.2, .15, .1}, Ground: Vec3{.04, .03, .02}},
	})
	ids := [7]uint64{0x100000001, 0x200000002, 0x300000003, 0x400000004, 0x500000005, 0x600000006, 0x700000007}
	positions := [7]float32{-.6, -.4, -.2, 0, .2, .4, .6}
	receivers := make([]Receiver, len(ids))

	for index, position := range positions {
		receivers[index] = gpuFixtureReceiver(ids[index], position)
	}

	return scene, receivers
}

func runSamplePoolTail(
	t *testing.T,
	ctx context.Context,
	packed GPUScene,
	tile GPUTile,
	profile bool,
	lanes uint64,
) ([]byte, GPUTraceCounts) {
	t.Helper()

	count := uint64(tile.Count)
	perLane := count * (gpuPathWords + gpuPathQueueCount) * gpuWordBytes
	fixed := uint64(64+12+len(packed.Data)+len(tile.Data)) + count*gpuResultWords*gpuWordBytes*2 + gpuPathHeaderBytes
	budget := fixed + lanes*perLane

	if profile {
		budget += count*gpuTraceCounterBytes + gpuWordBytes
	}

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{
		MemoryBytes: budget, TraceRays: profile,
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, runErr := worker.Run(ctx, packed, tile)
	counts := worker.TraceRayCounts()
	closeErr := worker.Close()

	if runErr != nil || closeErr != nil {
		t.Fatal("pool execution", profile, lanes, runErr, closeErr)
	}

	t.Logf("profiling=%v lanes=%d memory=%d counts=%+v", profile, lanes, budget, counts)

	return raw, counts
}

func assertSamplePoolTail(t *testing.T, raw, reference []byte, tile GPUTile) {
	t.Helper()

	if !bytes.Equal(raw, reference) {
		t.Fatal("ordered sample results changed across pool width or profiling")
	}

	for receiver := range tile.Count {
		if got := binary.LittleEndian.Uint32(raw[(receiver*gpuResultWords+33)*gpuWordBytes:]); got != tile.Options.Samples {
			t.Fatalf("receiver%d completed%d samples, want%d", receiver, got, tile.Options.Samples)
		}
	}
}
