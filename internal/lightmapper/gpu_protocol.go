//go:build !js

package lightmapper

import "math"

func gpuExpectedResultBytes(tile GPUTile) (int64, error) {
	words := gpuOutputRecordWords(tile)
	if words > (math.MaxInt64-1)/gpuWordBytes {
		return 0, ErrBounds
	}

	perReceiver := words * gpuWordBytes
	if uint64(tile.Count) > (math.MaxInt64-1)/perReceiver {
		return 0, ErrBounds
	}

	bytes := uint64(tile.Count) * perReceiver
	if bytes >= math.MaxInt64 {
		return 0, ErrBounds
	}

	return int64(bytes), nil
}
