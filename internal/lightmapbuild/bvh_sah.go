//go:build !js

package lightmapbuild

import (
	"fmt"
	"math"
	"sort"

	"github.com/karty-game/ashura/internal/lightmapdata"
)

const (
	sahBins          = 12
	sahChildren      = 2
	sahLeafTriangles = 8
	sahMedianDepth   = 32
)

type sahBuilder struct {
	triangles, scratch []triangle
	nodes              []bvhNode
	failure            error
}

type sahBucket struct {
	bounds

	count int
}
type sahSplit struct {
	axis, bin, count    int
	lower, extent, cost float64
}

// buildSAHBVH is used only by level preparation. Fixed adjacent child slots and
// the portable node/triangle layout are shared with the retained median builder.
func buildSAHBVH(triangles []triangle) ([]byte, []byte, error) {
	if len(triangles) == 0 {
		return nil, nil, errGeometry
	}

	builder := sahBuilder{triangles: triangles, scratch: make([]triangle, len(triangles)), nodes: []bvhNode{{}}}
	builder.build(0, 0, len(triangles), 0)

	if builder.failure != nil {
		return nil, nil, builder.failure
	}

	nodes, positions := packBVH(builder.nodes, triangles)

	return nodes, positions, nil
}

//nolint:gosec // Admitted triangle/node ranges are bounded below uint32 by MaxTriangles and MaxNodes.
func (builder *sahBuilder) build(index, start, end, depth int) {
	if builder.failure != nil {
		return
	}

	box, centers := emptyBounds(), emptyBounds()

	for _, triangle := range builder.triangles[start:end] {
		for _, point := range triangle.points {
			box.include(point)
		}

		centers.include(triangle.center)
	}

	builder.nodes[index].bounds = box
	if end-start <= sahLeafTriangles {
		builder.nodes[index].first = uint32(start)
		builder.nodes[index].count = uint32(end - start)

		return
	}

	middle := -1

	if depth < sahMedianDepth {
		split := chooseSAHSplit(builder.triangles[start:end], centers)
		if split.axis >= 0 {
			middle = builder.partition(start, end, split)
		}
	}

	if middle <= start || middle >= end {
		middle = medianTriangles(builder.triangles, start, end, centers)
	}

	if len(builder.nodes) > lightmapdata.MaxNodes-2 {
		builder.failure = fmt.Errorf("SAH visibility scene exceeds%d BVH nodes: %w", lightmapdata.MaxNodes, errGeometry)

		return
	}

	first := len(builder.nodes)
	builder.nodes[index].first = uint32(first)
	builder.nodes = append(builder.nodes, bvhNode{}, bvhNode{})
	builder.build(first, start, middle, depth+1)
	builder.build(first+1, middle, end, depth+1)
}

func medianTriangles(triangles []triangle, start, end int, centers bounds) int {
	axis := 0
	for candidate := range 3 {
		if centers.maximum[candidate]-centers.minimum[candidate] > centers.maximum[axis]-centers.minimum[axis] {
			axis = candidate
		}
	}

	sort.Slice(triangles[start:end], func(i, j int) bool {
		a, b := triangles[start+i], triangles[start+j]
		if a.center[axis] == b.center[axis] {
			return a.ordinal < b.ordinal
		}

		return a.center[axis] < b.center[axis]
	})

	return (start + end) / sahChildren
}

func (builder *sahBuilder) partition(start, end int, split sahSplit) int {
	left, right := start, start+split.count
	for _, triangle := range builder.triangles[start:end] {
		if sahBin(triangle.center[split.axis], split.lower, split.extent) <= split.bin {
			builder.scratch[left] = triangle
			left++
		} else {
			builder.scratch[right] = triangle
			right++
		}
	}

	copy(builder.triangles[start:end], builder.scratch[start:end])

	return start + split.count
}

func chooseSAHSplit(triangles []triangle, centers bounds) sahSplit {
	best := sahSplit{axis: -1, cost: math.Inf(1)}
	for axis := range 3 {
		lower, extent := float64(centers.minimum[axis]), float64(centers.maximum[axis])-float64(centers.minimum[axis])
		if extent <= 0 {
			continue
		}

		buckets := [sahBins]sahBucket{}
		for index := range buckets {
			buckets[index].bounds = emptyBounds()
		}

		for _, triangle := range triangles {
			bucket := &buckets[sahBin(triangle.center[axis], lower, extent)]

			bucket.count++
			for _, point := range triangle.points {
				bucket.include(point)
			}
		}

		candidate := splitBuckets(buckets)

		candidate.axis, candidate.lower, candidate.extent = axis, lower, extent
		if candidate.cost < best.cost {
			best = candidate
		}
	}

	return best
}

func splitBuckets(buckets [sahBins]sahBucket) sahSplit {
	suffix := [sahBins]sahBucket{}

	current := sahBucket{bounds: emptyBounds()}
	for index := sahBins - 1; index >= 0; index-- {
		includeBucket(&current, buckets[index])
		suffix[index] = current
	}

	left := sahBucket{bounds: emptyBounds()}
	best := sahSplit{axis: -1, cost: math.Inf(1)}

	for index := range sahBins - 1 {
		includeBucket(&left, buckets[index])

		right := suffix[index+1]
		if left.count < 2 || right.count < 2 {
			continue
		}

		cost := sahArea(left.bounds)*float64(left.count) + sahArea(right.bounds)*float64(right.count)
		if cost < best.cost {
			best.bin, best.count, best.cost = index, left.count, cost
		}
	}

	return best
}

func includeBucket(destination *sahBucket, source sahBucket) {
	if source.count == 0 {
		return
	}

	destination.count += source.count
	destination.include(source.minimum)
	destination.include(source.maximum)
}

func sahArea(box bounds) float64 {
	width, height, depth := float64(
		box.maximum[0],
	)-float64(
		box.minimum[0],
	), float64(
		box.maximum[1],
	)-float64(
		box.minimum[1],
	), float64(
		box.maximum[2],
	)-float64(
		box.minimum[2],
	)

	return width*height + width*depth + height*depth
}

func sahBin(center float32, lower, extent float64) int {
	return min(sahBins-1, max(0, int((float64(center)-lower)/extent*sahBins)))
}
