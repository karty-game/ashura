//go:build !js

package lightmapper

import "encoding/binary"

// Verification has one canonical padded-normal region and an optional complete
// per-receiver seed tail. Admit all ranges before a native worker sees the job.
func gpuValidateVerification(tile GPUTile) error {
	if len(tile.Data) < gpuTileHeaderWords*gpuWordBytes || tile.ProbeCount == 0 {
		return ErrInvalid
	}

	word := func(index int) uint64 { return uint64(binary.LittleEndian.Uint32(tile.Data[index*gpuWordBytes:])) }
	size := uint64(len(tile.Data) / gpuWordBytes)
	receivers := uint64(tile.Count)

	patchOffset := uint64(gpuTileHeaderWords) + receivers*gpuReceiverWords
	if word(0) != receivers || word(1) != gpuTileHeaderWords || word(2) != patchOffset || word(3) == 0 ||
		word(5) != uint64(tile.ProbeCount) {
		return ErrInvalid
	}

	if patchOffset > size || word(3) > (size-patchOffset)/gpuPatchWords {
		return ErrInvalid
	}

	probeOffset := patchOffset + word(3)*gpuPatchWords
	if word(4) != probeOffset || probeOffset > size {
		return ErrInvalid
	}

	components := uint64(tile.ProbeCount) * gpuRGBWords
	if receivers > (size-probeOffset)/components {
		return ErrInvalid
	}

	normalEnd := probeOffset + receivers*components
	if word(6) == 0 {
		if word(7) != 0 || normalEnd != size {
			return ErrInvalid
		}

		return nil
	}

	if word(6) != normalEnd || word(7) != receivers || receivers != size-normalEnd {
		return ErrInvalid
	}

	return nil
}
