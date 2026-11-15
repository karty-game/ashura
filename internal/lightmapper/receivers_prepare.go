package lightmapper

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"sort"
	"sync"
)

// PrepareReceivers integrates normalized UV1 support. UV1 and scene triangles
// have identical ordering. Output is sparse, sorted by pixel, with front sides;
// independent back-side charts must be provided as a separate receiver page.
func PrepareReceivers(triangles []Triangle, uv1 [][3]Vec2, width, height uint32) ([]Receiver, error) {
	return prepareReceiversWithWorkers(triangles, uv1, width, height, runtime.GOMAXPROCS(0))
}

//nolint:gosec // Each row boundary is between zero and the admitted uint32 height before conversion.
func prepareReceiversWithWorkers(triangles []Triangle, uv1 [][3]Vec2, width, height uint32, workers int) ([]Receiver, error) {
	if width == 0 || height == 0 || uint64(width)*uint64(height) > math.MaxUint32 || len(triangles) != len(uv1) {
		return nil, errReceiver
	}

	workers = max(1, workers)
	if uint64(workers) > uint64(height) {
		workers = int(height)
	}

	if workers == 1 {
		return prepareReceiverRows(triangles, uv1, width, height, 0, height)
	}

	type preparedRows struct {
		receivers []Receiver
		err       error
	}

	rows := make([]preparedRows, workers)

	var group sync.WaitGroup

	for index := range rows {
		first := uint32(uint64(height) * uint64(index) / uint64(workers))
		last := uint32(uint64(height) * uint64(index+1) / uint64(workers))

		group.Go(func() {
			rows[index].receivers, rows[index].err = prepareReceiverRows(triangles, uv1, width, height, first, last)
		})
	}

	group.Wait()

	count := 0

	for _, row := range rows {
		if row.err != nil {
			return nil, row.err
		}

		count += len(row.receivers)
	}

	output := make([]Receiver, 0, count)
	for _, row := range rows {
		output = append(output, row.receivers...)
	}

	return output, nil
}

// Each row domain visits original triangles and pieces in their original order.
// Different domains own disjoint pixels, so concatenating their sorted outputs
// preserves every footprint, accumulation order and stored floating-point bit.
func prepareReceiverRows(triangles []Triangle, uv1 [][3]Vec2, width, height, first, last uint32) ([]Receiver, error) {
	state := receiverRowBuilder{
		triangles: triangles, coordinates: uv1, width: width, height: height,
		pixels:    newReceiverPixelIndex(len(triangles), width, height, first, last),
		receivers: make([]Receiver, 0),
	}
	for index, triangleUV := range uv1 {
		pieces, err := receiverClipTriangleRows(uint32(index), triangleUV, width, height, first, last)
		if err != nil {
			return nil, fmt.Errorf("triangle %d (id %d), UV1 %v: %w", index, triangles[index].ID, triangleUV, err)
		}

		if len(pieces) == 0 {
			continue
		}

		frame := receiverTrianglePatchFrame(triangles[index])

		arena, fresh, err := receiverTrianglePatchArena(pieces, state.pixels)
		if err != nil {
			return nil, err
		}

		state.receivers = slices.Grow(state.receivers, fresh)
		for _, piece := range pieces {
			if err := state.appendPiece(index, piece, frame, &arena); err != nil {
				return nil, err
			}
		}
	}

	output := state.receivers
	for index := range output {
		receiver := &output[index]
		if err := receiverGuide(triangles, receiver); err != nil {
			return nil, fmt.Errorf("pixel %d guide: %w", receiver.Pixel, err)
		}
	}

	sort.Slice(output, func(i, j int) bool { return output[i].Pixel < output[j].Pixel })

	return output, nil
}
