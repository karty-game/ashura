package lightmapper

import "slices"

type surfaceBoundary struct {
	key     [6]uint32
	segment surfaceSegment
	count   int
}

// Most clipped luxels contain one polygon fan. Reusing a compact scratch list
// avoids two allocated hash tables per luxel. Larger arbitrary footprints use
// the original hash algorithm so preprocessing never becomes quadratic.
func surfaceReceiverBoundary(patches []ReceiverPatch, owner uint32, scratch []surfaceBoundary) ([]surfaceBoundary, error) {
	const linearEdges = 16

	scratch = scratch[:0]

	if len(patches) > linearEdges/3 {
		counts, records, err := surfacePatchSegments(patches, owner)
		if err != nil {
			return nil, err
		}

		for key, count := range counts {
			scratch = append(scratch, surfaceBoundary{key: key, segment: records[key], count: count})
		}
	} else {
		for _, patch := range patches {
			var err error

			scratch, err = surfacePatchBoundary(patch, owner, scratch)
			if err != nil {
				return nil, err
			}
		}
	}

	slices.SortFunc(scratch, func(first, second surfaceBoundary) int {
		for axis := range 6 {
			if first.key[axis] < second.key[axis] {
				return -1
			}

			if first.key[axis] > second.key[axis] {
				return 1
			}
		}

		return 0
	})

	return scratch, nil
}

func surfacePatchBoundary(patch ReceiverPatch, owner uint32, scratch []surfaceBoundary) ([]surfaceBoundary, error) {
	normal := patch.GeometricNormal
	if Dot(normal, normal) == 0 {
		normal = Normalize(Cross(Sub(patch.Positions[1], patch.Positions[0]), Sub(patch.Positions[2], patch.Positions[0])))
	}

	center := Scale(Add(patch.Positions[0], Add(patch.Positions[1], patch.Positions[2])), 1.0/3)
	for corner := range 3 {
		first, second := patch.Positions[corner], patch.Positions[(corner+1)%3]
		if !validVec(first) || !validVec(second) || !validVec(normal) {
			return nil, ErrInvalid
		}

		if first == second {
			continue
		}

		segment := surfaceSegment{owner: owner, a: first, b: second, center: center, normal: normal}
		scratch = surfaceAppendBoundary(scratch, surfaceSegmentKey(first, second), segment)
	}

	return scratch, nil
}

func surfaceAppendBoundary(scratch []surfaceBoundary, key [6]uint32, segment surfaceSegment) []surfaceBoundary {
	for index := range scratch {
		if scratch[index].key == key {
			scratch[index].count++
			scratch[index].segment = segment

			return scratch
		}
	}

	return append(scratch, surfaceBoundary{key: key, count: 1, segment: segment})
}
