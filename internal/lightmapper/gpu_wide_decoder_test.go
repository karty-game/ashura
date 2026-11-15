//go:build !js

//nolint:varnamelen,gocognit,nestif // An independent packed-mask decoder mirrors the documented bit fields, not the producer hierarchy.
package lightmapper

import (
	"math/bits"
	"testing"
)

// Decode the actual packed masks/relative child addresses in all octants. This
// does not use the source builder nodes or frontier to discover the topology.
func assertPackedGPUWideCoverage(t *testing.T, b *gpuWideBuilder) {
	t.Helper()

	for octant := range uint32(8) {
		type group struct{ base, mask uint32 }

		current := group{mask: 0x80000000}

		var stack []group

		seenNodes := make([]bool, len(b.packed)/20)
		seenTriangles := make([]bool, len(b.oldOrder))

		for {
			if current.mask > 0x00ffffff {
				mask := current.mask
				bit := gpuOffsetWord(31 - bits.LeadingZeros32(mask))

				current.mask &^= 1 << bit
				if current.mask > 0x00ffffff {
					stack = append(stack, current)
				}

				slot := (bit - 24) ^ (7 - octant)

				index := current.base + gpuOffsetWord(bits.OnesCount32(mask&((1<<slot)-1)))
				if index >= gpuOffsetWord(len(seenNodes)) || seenNodes[index] {
					t.Fatalf("octant %d invalid/repeated node %d", octant, index)
				}

				seenNodes[index] = true
				at := int(index) * 20

				var hits uint32

				for child := range 8 {
					meta := (b.packed[at+6+child/4] >> uint(child%4*8)) & 255
					if meta == 0 {
						continue
					}

					inner := b.packed[at+3]&(1<<uint(24+child)) != 0

					offset := meta & 31
					if inner {
						if offset != uint32(24+child) || meta>>5 != 1 {
							t.Fatal("invalid internal metadata")
						}

						offset ^= 7 - octant
					} else if offset >= 24 || (meta>>5 != 1 && meta>>5 != 3 && meta>>5 != 7) {
						t.Fatal("invalid leaf metadata")
					}

					hits |= (meta >> 5) << offset
				}

				triangleBits := hits & 0x00ffffff

				if b.packed[at+5]&3 != 0 {
					t.Fatal("unaligned leaf base")
				}

				for triangleBits != 0 {
					bit := gpuOffsetWord(bits.TrailingZeros32(triangleBits))
					triangleBits &= triangleBits - 1

					slot := b.packed[at+5]/4 + bit
					if slot >= gpuOffsetWord(len(b.order)) {
						t.Fatal("leaf outside order")
					}

					id := b.order[slot]
					if id >= gpuOffsetWord(len(seenTriangles)) || seenTriangles[id] {
						t.Fatalf("octant %d invalid/repeated primitive %d", octant, id)
					}

					seenTriangles[id] = true
				}

				current = group{b.packed[at+4], hits&0xff000000 | b.packed[at+3]>>24}
			}

			if current.mask > 0x00ffffff {
				continue
			}

			if len(stack) == 0 {
				break
			}

			current = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}

		for id, seen := range seenNodes {
			if !seen {
				t.Fatalf("octant %d unreachable node %d", octant, id)
			}
		}

		for id, seen := range seenTriangles {
			if !seen {
				t.Fatalf("octant %d missing primitive %d", octant, id)
			}
		}
	}
}
