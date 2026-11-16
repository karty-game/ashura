//go:build !js

package meshopt

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestOptimizeUVPreservesCornersAndRemapsDuplicateTriangles(t *testing.T) {
	t.Parallel()

	part := gridPart()
	part.Indices = append(part.Indices, part.Indices[:12]...)
	source := modeldata.Document{Parts: []modeldata.Part{part, fixturePart()}}

	uvs := make([][]byte, len(source.Parts))
	for index, original := range source.Parts {
		for vertex := range len(original.Vertices) / 32 {
			uvs[index] = binary.LittleEndian.AppendUint64(uvs[index], uint64(vertex+1))
		}
	}

	output, remapped, triangles, err := OptimizeUV(t.Context(), source, uvs)
	if err != nil {
		t.Fatal(err)
	}

	base := 0

	for index, original := range source.Parts {
		seen := make(map[uint32]bool)

		for triangle := range len(original.Indices) / 12 {
			destination := triangles[base+triangle] - uint32(base)
			if seen[destination] || int(destination) >= len(original.Indices)/12 {
				t.Fatal("triangle mapping is not a permutation")
			}

			seen[destination] = true

			for corner := range 3 {
				before := int(binary.LittleEndian.Uint32(original.Indices[triangle*12+corner*4:]))

				after := int(binary.LittleEndian.Uint32(output.Parts[index].Indices[int(destination)*12+corner*4:]))
				if !bytes.Equal(original.Vertices[before*32:(before+1)*32], output.Parts[index].Vertices[after*32:(after+1)*32]) ||
					!bytes.Equal(uvs[index][before*8:(before+1)*8], remapped[index][after*8:(after+1)*8]) {
					t.Fatal("changed oriented geometry or UV1 corner")
				}
			}
		}

		base += len(original.Indices) / 12
	}

	if _, _, _, err := OptimizeUV(t.Context(), source, nil); err == nil {
		t.Fatal("accepted missing UV streams")
	}

	uvs[0] = uvs[0][:len(uvs[0])-1]
	if _, _, _, err := OptimizeUV(t.Context(), source, uvs); err == nil {
		t.Fatal("accepted truncated UV stream")
	}
}
