package lightmapper

import (
	"reflect"
	"testing"
)

func TestParallelSurfaceSegmentsRetainCanonicalBoundaryOrder(t *testing.T) {
	t.Parallel()
	scene, original := surfaceEdgeFixture(t)

	receivers := make([]Receiver, 0, len(original)*12)
	for group := range 12 {
		for _, receiver := range original {
			receiver.ID = uint64(len(receivers) + 1)
			receiver.Position[0] += float32(group) * 2

			receiver.Patches = append([]ReceiverPatch(nil), receiver.Patches...)
			for patch := range receiver.Patches {
				for corner := range 3 {
					receiver.Patches[patch].Positions[corner][0] += float32(group) * 2
				}
			}

			receivers = append(receivers, receiver)
		}
	}

	var expected []surfaceSegment

	var scratch []surfaceBoundary

	for owner, receiver := range receivers {
		patches, err := ReceiverPatches(scene, receiver)
		if err != nil {
			t.Fatal(err)
		}

		scratch, err = surfaceReceiverBoundary(patches, uint32(owner), scratch)
		if err != nil {
			t.Fatal(err)
		}

		for _, boundary := range scratch {
			if boundary.count == 1 {
				expected = append(expected, boundary.segment)
			}
		}
	}

	for _, workers := range []int{1, 2, 7} {
		segments, err := surfaceSegmentsWithWorkers(t.Context(), scene, receivers, workers)
		if err != nil || !reflect.DeepEqual(segments, expected) {
			t.Fatal("parallel footprint preparation changed canonical order", workers, err)
		}
	}

	receivers[len(receivers)-1].ID = receivers[0].ID
	if segments, err := surfaceSegmentsWithWorkers(t.Context(), scene, receivers, 7); err == nil || segments != nil {
		t.Fatal("duplicate identity escaped complete preflight", err)
	}
}
