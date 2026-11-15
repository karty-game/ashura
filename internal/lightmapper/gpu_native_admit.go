//go:build !js

//nolint:mnd // Buffer layouts, flag words and dispatch widths mirror the production WGSL ABI.
package lightmapper

import (
	"encoding/binary"
	"math"
	"strings"
	"unicode/utf8"

	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

type nativeBatchSizes struct {
	paths, output, readback uint64
	pool                    uint32
}

func nativeBatchAdmission(scene GPUScene, tile GPUTile, budget uint64, limits wgpu.Limits, profile bool) (nativeBatchSizes, error) {
	if err := nativeBatchPacket(scene, tile); err != nil {
		return nativeBatchSizes{}, err
	}

	count := uint64(tile.Count)

	sizes := nativeBatchSizes{paths: gpuPathHeaderBytes + count*(gpuPathRecordWords(tile)+gpuPathQueueCount)*gpuWordBytes,
		output: count * gpuOutputRecordWords(tile) * gpuWordBytes, pool: 1}
	if tile.Mode == gpuModeReconstruct {
		sizes.paths = gpuPathMinimumBytes
	}

	sizes.readback = sizes.output
	if profile && tile.Mode != gpuModeReconstruct {
		sizes.readback += count*gpuTraceCounterBytes + gpuWordBytes
	}

	if sizes.readback > limits.MaxBufferSize {
		return nativeBatchSizes{}, ErrBounds
	}

	total := uint64(64+12) + sizes.readback
	for _, size := range []uint64{uint64(len(scene.Data)), uint64(len(tile.Data)), sizes.paths, sizes.output} {
		if size > limits.MaxBufferSize || size > limits.MaxStorageBufferBindingSize || size > math.MaxUint64-total {
			return nativeBatchSizes{}, ErrBounds
		}

		total += size
	}

	if total > budget || (count+63)/64 > uint64(limits.MaxComputeWorkgroupsPerDimension) || limits.MaxStorageBuffersPerShaderStage < 4 {
		return nativeBatchSizes{}, ErrBounds
	}

	if tile.Mode == "" {
		perLane := sizes.paths - gpuPathHeaderBytes
		capacity := min(
			1+(budget-total)/perLane,
			(limits.MaxBufferSize-gpuPathHeaderBytes)/perLane,
			(limits.MaxStorageBufferBindingSize-gpuPathHeaderBytes)/perLane,
			uint64(limits.MaxComputeWorkgroupsPerDimension)*64/count,
			uint64(tile.Options.Samples),
		)
		sizes.pool = uint32(capacity) // #nosec G115 -- capacity is bounded by admitted sample count <= 2^24.
		sizes.paths = gpuPathHeaderBytes + perLane*capacity
	}

	return sizes, nil
}

func nativeBatchPacket(scene GPUScene, tile GPUTile) error {
	if err := gpuValidatePackets(scene, tile); err != nil {
		return err
	}

	if !utf8.ValidString(scene.Key) || strings.ContainsRune(scene.Key, 0) || len(tile.Data) < 32 || tile.Count > 1<<20 ||
		tile.Options.Samples == 0 ||
		tile.Options.Samples > 1<<24 ||
		tile.Options.RouletteDepth > 32 ||
		tile.Options.Outputs & ^(OutputRGB|OutputRNM) != 0 ||
		(tile.Mode != gpuModeVerify && tile.ProbeCount != 0) {
		return ErrInvalid
	}

	if tile.Mode == gpuModeReconstruct {
		if binary.LittleEndian.Uint32(tile.Data) != 0x4b4c5251 || binary.LittleEndian.Uint32(tile.Data[4:]) != 1 ||
			binary.LittleEndian.Uint32(tile.Data[8:]) != tile.Count {
			return ErrInvalid
		}
	} else if binary.LittleEndian.Uint32(tile.Data) != tile.Count {
		return ErrInvalid
	}

	return nil
}
