//go:build !js

//nolint:varnamelen,mnd // Packed BVH8 fields use their specified byte/bit positions and geometric axes.
package lightmapper

import (
	"encoding/binary"
	"math"
	"math/bits"
)

type gpuWideAdmission struct {
	data                                     []byte
	nodes, triangles, source, count          uint32
	seenNodes, seenSlots, seenIDs, seenRanks []bool
	depth                                    int
}

func (a *gpuWideAdmission) word(at uint32) uint32 {
	return binary.LittleEndian.Uint32(a.data[int(at)*gpuWordBytes:])
}
func (a *gpuWideAdmission) scalar(at uint32) float32 { return math.Float32frombits(a.word(at)) }

func gpuValidateWideScene(data []byte) bool {
	if len(data) < gpuSceneHeaderWords*gpuWordBytes || !gpuSceneAligned(data) {
		return false
	}

	a := gpuWideAdmission{data: data}
	a.nodes = a.word(2)
	a.triangles = a.word(33)
	a.source = a.word(6)
	a.count = a.word(5)
	a.seenIDs = make([]bool, int(a.count))
	a.seenRanks = make([]bool, int(a.count))
	a.seenSlots = make([]bool, int(a.count))

	a.seenNodes = make([]bool, int(a.word(3)))
	for slot := range a.count {
		if !a.admitTriangle(slot) {
			return false
		}
	}

	if len(a.seenNodes) > 0 {
		if _, okay := a.admitNode(0, 1); !okay {
			return false
		}

		if a.depth != int(a.word(gpuWideDepthWord)) {
			return false
		}

		for _, seen := range a.seenNodes {
			if !seen {
				return false
			}
		}

		for _, seen := range a.seenSlots {
			if !seen {
				return false
			}
		}
	}

	return true
}

func (a *gpuWideAdmission) admitTriangle(slot uint32) bool {
	at := a.triangles + slot*12

	id, rank := a.word(at+7), a.word(at+11)
	if id >= a.count || rank >= a.count || a.seenIDs[id] || a.seenRanks[rank] {
		return false
	}

	a.seenIDs[id], a.seenRanks[rank] = true, true
	source := a.source + id*gpuTriangleWords

	var p [3]Vec3

	for vertex := range 3 {
		for axis := range 3 {
			p[vertex][axis] = a.scalar(source + uint32(vertex*4+axis))
			if !finite(p[vertex][axis]) {
				return false
			}
		}
	}

	e1, e2 := Sub(p[1], p[0]), Sub(p[2], p[0])
	for axis := range 3 {
		if a.word(at+uint32(axis)) != math.Float32bits(p[0][axis]) || a.word(at+4+uint32(axis)) != math.Float32bits(e1[axis]) ||
			a.word(at+8+uint32(axis)) != math.Float32bits(e2[axis]) {
			return false
		}
	}

	return a.word(at+3) == math.Float32bits(Length(Cross(e1, e2)))
}

func (a *gpuWideAdmission) triangleBounds(slot uint32) gpuWideBox {
	id := a.word(a.triangles + slot*12 + 7)
	source := a.source + id*gpuTriangleWords
	bound := emptyGPUWideBox()

	for vertex := range 3 {
		for axis := range 3 {
			x := a.scalar(source + uint32(vertex*4+axis))
			bound.lo[axis] = min(bound.lo[axis], x)
			bound.hi[axis] = max(bound.hi[axis], x)
		}
	}

	return bound
}

//nolint:gocognit,nestif,cyclop,funlen // One recursive admission joins topology, primitive coverage and source-bound enclosure before any node is accepted.
func (a *gpuWideAdmission) admitNode(index uint32, depth int) (gpuWideBox, bool) {
	if index >= a.word(3) || a.seenNodes[index] || depth > gpuWideMaximumDepth {
		return gpuWideBox{}, false
	}

	a.seenNodes[index] = true
	a.depth = max(a.depth, depth)
	at := a.nodes + index*gpuNodeWords
	encoding := a.word(at + 3)
	inner := encoding >> 24
	first := a.word(at + 4)

	childCount := gpuWideBitCount(inner)
	if first > a.word(3) || childCount > a.word(3)-first || childCount == 0 && first != 0 ||
		a.word(at+5)&3 != 0 ||
		a.word(at+5)/4 > a.count {
		return gpuWideBox{}, false
	}

	bound := emptyGPUWideBox()
	present := false

	for child := range uint32(8) {
		meta := (a.word(at+6+child/4) >> (child % 4 * 8)) & 255

		isInner := inner&(1<<child) != 0
		if meta == 0 {
			if isInner {
				return gpuWideBox{}, false
			}

			continue
		}

		present = true
		offset, mask := meta&31, meta>>5

		var childBound gpuWideBox

		if isInner {
			if offset != 24+child || mask != 1 {
				return gpuWideBox{}, false
			}

			relative := gpuWideBitCount(inner & ((1 << child) - 1))

			var okay bool

			childBound, okay = a.admitNode(first+relative, depth+1)
			if !okay {
				return gpuWideBox{}, false
			}
		} else {
			if offset >= 24 || mask != 1 && mask != 3 && mask != 7 {
				return gpuWideBox{}, false
			}

			start := a.word(at+5)/4 + offset

			count := gpuWideBitCount(mask)
			if start > a.count || count > a.count-start {
				return gpuWideBox{}, false
			}

			childBound = emptyGPUWideBox()

			for slot := start; slot < start+count; slot++ {
				if a.seenSlots[slot] {
					return gpuWideBox{}, false
				}

				a.seenSlots[slot] = true
				childBound = mergeGPUWideBox(childBound, a.triangleBounds(slot))
			}
		}

		if !a.cellContains(at, encoding, child, childBound) {
			return gpuWideBox{}, false
		}

		bound = mergeGPUWideBox(bound, childBound)
	}

	return bound, present
}

func (a *gpuWideAdmission) cellContains(at, encoding, child uint32, bound gpuWideBox) bool {
	for axis := range uint32(3) {
		origin := a.scalar(at + axis)

		exponent := (encoding >> (axis * 8)) & 255
		if !finite(origin) || exponent < 1 || exponent > 253 {
			return false
		}

		low := (a.word(at+8+axis*2+child/4) >> (child % 4 * 8)) & 255

		high := (a.word(at+14+axis*2+child/4) >> (child % 4 * 8)) & 255
		if low > high {
			return false
		}

		lo := float32(float64(origin) + math.Ldexp(float64(low), int(exponent)-127))

		hi := float32(float64(origin) + math.Ldexp(float64(high), int(exponent)-127))
		if lo > bound.lo[axis] || hi < bound.hi[axis] {
			return false
		}
	}

	return true
}

func gpuWideBitCount(value uint32) uint32 { return uint32(bits.OnesCount32(value)) } // #nosec G115 -- a32-bit mask has at most32 set bits.
