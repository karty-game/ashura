package lightmapper

import (
	"context"
	"math"
	"runtime"
)

func surfaceSegmentsContext(ctx context.Context, scene *PreparedScene, receivers []Receiver) ([]surfaceSegment, error) {
	return surfaceSegmentsWithWorkers(ctx, scene, receivers, runtime.GOMAXPROCS(0))
}

// Admit identities and vectors before workers build independent footprints.
// Contiguous ranges and the ordered merge retain the exact boundary order used
// by contact ambiguity admission, regardless of worker scheduling.
func surfaceSegmentsWithWorkers(
	ctx context.Context, scene *PreparedScene, receivers []Receiver, requestedWorkers int,
) ([]surfaceSegment, error) {
	ids := make(map[uint64]bool, len(receivers))
	for _, receiver := range receivers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if ids[receiver.ID] || !validVec(receiver.Position) || !validVec(receiver.GeometricNormal) {
			return nil, ErrInvalid
		}

		ids[receiver.ID] = true
	}

	workers := min(requestedWorkers, len(receivers))
	parts := make([][]surfaceSegment, workers)
	errors := make([]error, workers)
	surfaceContactRanges(len(receivers), workers, func(worker, start, end int) {
		parts[worker], errors[worker] = surfaceSegmentRange(ctx, scene, receivers, start, end)
	})

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var count uint64

	for worker, part := range parts {
		if errors[worker] != nil {
			return nil, errors[worker]
		}

		count += uint64(len(part))
		if count > math.MaxUint32 || count > uint64(math.MaxInt) {
			return nil, ErrBounds
		}
	}

	if count == 0 {
		return nil, nil
	}

	output := make([]surfaceSegment, 0, int(count))
	for _, part := range parts {
		output = append(output, part...)
	}

	return output, nil
}

//nolint:gosec // G115: complete receiver admission bounds each parallel owner range below MaxUint32.
func surfaceSegmentRange(
	ctx context.Context, scene *PreparedScene, receivers []Receiver, start, end int,
) ([]surfaceSegment, error) {
	capacity := end - start

	const commonBoundaryCount = 4
	if capacity <= math.MaxInt/commonBoundaryCount {
		capacity *= commonBoundaryCount
	}

	output := make([]surfaceSegment, 0, capacity)

	var scratch []surfaceBoundary

	for owner := start; owner < end; owner++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		patches, err := ReceiverPatches(scene, receivers[owner])
		if err != nil {
			return nil, err
		}

		scratch, err = surfaceReceiverBoundary(patches, uint32(owner), scratch)
		if err != nil {
			return nil, err
		}

		for _, boundary := range scratch {
			if boundary.count == 1 {
				output = append(output, boundary.segment)
			}
		}
	}

	return output, nil
}
