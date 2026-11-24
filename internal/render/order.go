package render

import (
	"slices"

	"github.com/karty-game/ashura/internal/meshdata"
)

type drawOrderKey struct {
	bounds meshdata.Bounds
	matrix [16]float32
	id     uint32
}

type orderedDraw struct {
	index  int
	depth  float32
	bucket int
}

// OpaqueOrder caches the color-pass order independently of residency and shadows.
type OpaqueOrder struct {
	view    [16]float32
	keys    []drawOrderKey
	entries []orderedDraw
	indices []int
}

// Prepare groups consecutive eligible opaque draws into stable depth buckets.
// Cutouts, impostors and unbounded draws retain their positions as barriers.
func (order *OpaqueOrder) Prepare(frame Frame) []int {
	if len(frame.Draws) < 2 || frame.ViewProjection == ([16]float32{}) {
		return nil
	}

	dirty := order.view != frame.ViewProjection || len(order.keys) != len(frame.Draws)
	if len(order.keys) != len(frame.Draws) {
		order.keys = slices.Grow(order.keys[:0], len(frame.Draws))[:len(frame.Draws)]
	}

	for index, draw := range frame.Draws {
		key := drawOrderKey{id: draw.ID}
		if !draw.AlphaMask && draw.Impostor == nil && !draw.Billboard && len(draw.Instances) <= 1 {
			key.bounds, key.matrix = draw.localBounds(), instanceAt(draw, 0).Model
		}

		dirty = dirty || order.keys[index] != key
		order.keys[index] = key
	}

	if !dirty {
		return order.indices
	}

	order.view = frame.ViewProjection
	order.entries = slices.Grow(order.entries[:0], len(frame.Draws))[:len(frame.Draws)]
	order.indices = slices.Grow(order.indices[:0], len(frame.Draws))[:len(frame.Draws)]
	start := 0
	minimum, maximum := float32(0), float32(0)

	for index, key := range order.keys {
		order.entries[index] = orderedDraw{index: index}

		depth, eligible := order.depth(key)
		if !eligible {
			order.sortRun(start, index, minimum, maximum)
			start = index + 1

			continue
		}

		if start == index {
			minimum, maximum = depth, depth
		} else {
			minimum, maximum = min(minimum, depth), max(maximum, depth)
		}

		order.entries[index].depth = depth
	}

	order.sortRun(start, len(order.entries), minimum, maximum)

	for index, entry := range order.entries {
		order.indices[index] = entry.index
	}

	return order.indices
}

func (order *OpaqueOrder) depth(key drawOrderKey) (float32, bool) {
	if !key.bounds.Valid {
		return 0, false
	}

	center := transformPoint(key.matrix, midpoint(key.bounds))
	view := order.view
	depth := view[2]*center[0] + view[6]*center[1] + view[10]*center[2]

	return depth, pointFinite(depth)
}

func (order *OpaqueOrder) sortRun(start, end int, minimum, maximum float32) {
	const buckets = 8

	span := float64(maximum) - float64(minimum)
	if end-start < 2 || span == 0 {
		return
	}

	run := order.entries[start:end]
	for index := range run {
		run[index].bucket = min(buckets-1, int((float64(run[index].depth)-float64(minimum))*buckets/span))
	}

	slices.SortStableFunc(run, func(a, b orderedDraw) int { return a.bucket - b.bucket })
}
