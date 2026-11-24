//go:build !js

package lightmapper

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func nativeAdmissionFixture(t *testing.T, count, samples uint32) (GPUScene, GPUTile, wgpu.Limits) {
	t.Helper()
	prepared := gpuFixtureScene(t, Scene{})

	scene, err := PackGPUScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	tile := GPUTile{
		Count:   count,
		Data:    make([]byte, 32),
		Options: Options{Samples: samples, Bounces: 3, Seed: 29, RouletteDepth: 3, Outputs: OutputRGB | OutputRNM},
	}
	binary.LittleEndian.PutUint32(tile.Data, count)

	limits := wgpu.Limits{
		MaxBufferSize:                    1 << 30,
		MaxStorageBufferBindingSize:      1 << 30,
		MaxStorageBuffersPerShaderStage:  4,
		MaxComputeWorkgroupsPerDimension: 65535,
	}

	return scene, tile, limits
}
func nativeAdmissionTotal(scene GPUScene, tile GPUTile, sizes nativeBatchSizes) uint64 {
	return uint64(76+len(scene.Data)+len(tile.Data)) + sizes.paths + sizes.output + sizes.readback
}
func TestNativeGoAdmissionRetainsExactMemoryAndDispatchBounds(t *testing.T) {
	t.Parallel()
	scene, tile, limits := nativeAdmissionFixture(t, 17, 1)

	sizes, err := nativeBatchAdmission(scene, tile, 1<<30, limits, false)
	if err != nil || sizes.pool != 1 {
		t.Fatal(sizes, err)
	}

	minimum := nativeAdmissionTotal(scene, tile, sizes)
	lane := uint64(tile.Count) * 520

	tile.Options.Samples = 32
	for _, entry := range []struct {
		budget uint64
		pool   uint32
	}{
		{minimum - 1, 0}, {minimum, 1}, {minimum + lane - 1, 1}, {minimum + lane, 2},
	} {
		sizes, err = nativeBatchAdmission(scene, tile, entry.budget, limits, false)
		if entry.pool == 0 {
			if !errors.Is(err, ErrBounds) {
				t.Fatal("under budget admitted", err)
			}
		} else if err != nil || sizes.pool != entry.pool {
			t.Fatal(entry, sizes, err)
		}
	}

	for _, kind := range []string{"storage", "buffer", "dispatch"} {
		bound := limits

		switch kind {
		case "storage":
			bound.MaxStorageBufferBindingSize = 16 + lane*3
		case "buffer":
			bound.MaxBufferSize = 16 + lane*3
		case "dispatch":
			bound.MaxComputeWorkgroupsPerDimension = 1
		}

		sizes, err = nativeBatchAdmission(scene, tile, 1<<30, bound, false)
		if err != nil || sizes.pool != 3 {
			t.Fatal(kind, sizes, err)
		}
	}

	sizes, err = nativeBatchAdmission(scene, tile, minimum+lane, limits, true)
	if err != nil || sizes.pool != 1 {
		t.Fatal("profiling omitted from pool budget", sizes, err)
	}
}
func TestNativeGoAdmissionMixedBudgetsKeepMaximalBoundedPool(t *testing.T) {
	t.Parallel()
	scene, tile, limits := nativeAdmissionFixture(t, 17, 1)

	random := uint32(29)
	for range 10000 {
		random ^= random << 13
		random ^= random >> 17
		random ^= random << 5
		tile.Count = 1 + random%((1<<20)-1)
		binary.LittleEndian.PutUint32(tile.Data, tile.Count)
		tile.Options.Samples = 1 + (random>>20)%32
		budget := uint64(64+random%961) << 20

		sizes, err := nativeBatchAdmission(scene, tile, budget, limits, random&1 != 0)
		if errors.Is(err, ErrBounds) {
			continue
		}

		if err != nil {
			t.Fatal(err)
		}

		if sizes.pool < 1 || sizes.pool > tile.Options.Samples || sizes.paths != 16+uint64(tile.Count)*uint64(sizes.pool)*520 ||
			nativeAdmissionTotal(
				scene,
				tile,
				sizes,
			) > budget || sizes.paths > limits.MaxBufferSize || sizes.paths > limits.MaxStorageBufferBindingSize ||
			(uint64(tile.Count)*uint64(sizes.pool)+63)/64 > uint64(limits.MaxComputeWorkgroupsPerDimension) {
			t.Fatal("admitted bounds", sizes)
		}

		if sizes.pool < tile.Options.Samples {
			next := sizes

			next.paths += uint64(tile.Count) * 520
			if nativeAdmissionTotal(scene, tile, next) <= budget && next.paths <= limits.MaxBufferSize &&
				next.paths <= limits.MaxStorageBufferBindingSize &&
				(uint64(tile.Count)*uint64(sizes.pool+1)+63)/64 <= uint64(limits.MaxComputeWorkgroupsPerDimension) {
				t.Fatal("pool unnecessarily capped", sizes)
			}
		}
	}
}
func TestNativeGoAdmissionRejectsMalformedOptionsBeforeAllocation(t *testing.T) {
	t.Parallel()

	scene, tile, limits := nativeAdmissionFixture(t, 17, 32)
	for _, mutate := range []func(*GPUScene, *GPUTile){
		func(s *GPUScene, _ *GPUTile) { s.Key = "bad\x00key" },
		func(_ *GPUScene, v *GPUTile) { v.Count = 0 },
		func(_ *GPUScene, v *GPUTile) { v.Options.Samples = math.MaxUint32 },
		func(_ *GPUScene, v *GPUTile) { v.Options.RouletteDepth = 33 },
		func(_ *GPUScene, v *GPUTile) { v.Options.Outputs = 4 },
		func(_ *GPUScene, v *GPUTile) { v.ProbeCount = 1 },
	} {
		s, v := scene, tile
		mutate(&s, &v)

		if _, err := nativeBatchAdmission(s, v, 1<<30, limits, false); err == nil {
			t.Fatal("malformed input admitted")
		}
	}
}
