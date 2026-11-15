//go:build !js

package lightmapper

import (
	"encoding/binary"
	"slices"
	"testing"
)

func TestGPUWidePreservesOriginalRightFirstPriority(t *testing.T) {
	t.Parallel()
	// Source numbering deliberately differs from both order and packed BVH8.
	scene := &PreparedScene{
		nodes: []BVHNode{{Left: 4, Right: 1}, {Left: 3, Right: 2}, {First: 0, Count: 1}, {First: 1, Count: 1}, {First: 2, Count: 1}},
		order: []uint32{0, 1, 2},
	}
	if got := gpuGeometryPriorities(scene); !slices.Equal(got, []uint32{0, 1, 2}) {
		t.Fatal("changed right-first priorities", got)
	}

	prepared := gpuWideTestScene(t, 73)

	packed, err := PackGPUScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	original := gpuGeometryPriorities(prepared)

	expected := make([]uint32, len(original))
	for slot, id := range prepared.order {
		expected[id] = original[slot]
	}

	word := func(at int) uint32 { return binary.LittleEndian.Uint32(packed.Data[at*4:]) }

	first := int(word(33))
	for slot := range original {
		at := first + slot*12

		id, priority := word(at+7), word(at+11)
		if priority != expected[id] {
			t.Fatal("changed primitive priority", id, priority, expected[id])
		}
	}
}
