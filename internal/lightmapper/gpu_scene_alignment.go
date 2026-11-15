//go:build !js

package lightmapper

import "encoding/binary"

// This constant-time envelope runs for every job. The complete immutable graph
// is checked by its producer and once again before its first native upload.
//
//nolint:cyclop // Admit the complete constant-size geometry envelope atomically, before offsets become allocation or shader addresses.
func gpuSceneAligned(data []byte) bool {
	if len(data) < gpuSceneHeaderWords*gpuWordBytes {
		return false
	}

	words := uint64(len(data) / gpuWordBytes)
	word := func(at int) uint64 { return uint64(binary.LittleEndian.Uint32(data[at*gpuWordBytes:])) }

	nodes, nodeCount, triangles, triangleCount := word(2), word(3), word(33), word(5)
	if words > gpuMaximumBytes/gpuWordBytes || nodes != gpuSceneHeaderWords ||
		nodeCount > triangleCount || triangleCount > 1<<20 || nodeCount > (words-nodes)/gpuNodeWords ||
		word(6) != nodes+nodeCount*gpuNodeWords || word(6) > triangles || triangleCount > (triangles-word(6))/gpuTriangleWords ||
		word(8) != word(6)+triangleCount*gpuTriangleWords ||
		triangles&3 != 0 || triangles > words || triangleCount > (words-triangles)/12 || triangles+triangleCount*12 != words ||
		word(4) != 0 || word(34) != 0 || word(gpuWideModeWord) > 1 {
		return false
	}

	if triangleCount <= 3 || word(gpuWideModeWord) == 1 {
		if nodeCount != 0 || word(gpuWideDepthWord) != 0 {
			return false
		}
	} else if nodeCount == 0 || word(gpuWideDepthWord) == 0 || word(gpuWideDepthWord) > gpuWideMaximumDepth {
		return false
	}

	for at := 38; at < gpuSceneHeaderWords; at++ {
		if word(at) != 0 {
			return false
		}
	}

	return true
}
