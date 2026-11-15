//go:build !js

//nolint:varnamelen,mnd,gocognit,cyclop,funlen,nestif // Bounded SAH dynamic programming and quantized octant assignment keep conventional geometric indices together.
package lightmapper

import (
	"math"
	"sort"
)

const gpuWideSAHBins = 12

type gpuWideSAH struct {
	b       *gpuWideBuilder
	boxes   []gpuWideBox
	centers [][3]float64
}

type gpuWideBin struct {
	bounds gpuWideBox
	count  int
}

func mergeGPUWideBox(a, b gpuWideBox) gpuWideBox {
	for k := range 3 {
		a.lo[k] = min(a.lo[k], b.lo[k])
		a.hi[k] = max(a.hi[k], b.hi[k])
	}

	return a
}

func (s *gpuWideSAH) prepare() {
	s.boxes = make([]gpuWideBox, len(s.b.oldOrder))

	s.centers = make([][3]float64, len(s.b.oldOrder))
	for _, id := range s.b.oldOrder {
		v := s.b.bounds([]uint32{id})

		s.boxes[id] = v
		for k := range 3 {
			s.centers[id][k] = (float64(v.lo[k]) + float64(v.hi[k])) * .5
		}
	}
}

func gpuWideBinIndex(center, low, extent float64) int {
	return min(gpuWideSAHBins-1, max(0, int((center-low)/extent*gpuWideSAHBins)))
}

func (s *gpuWideSAH) build(ids []uint32, depth int) int {
	index := len(s.b.nodes)
	bound := emptyGPUWideBox()
	centroidLow := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	centroidHigh := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}

	for _, id := range ids {
		bound = mergeGPUWideBox(bound, s.boxes[id])
		for k, value := range s.centers[id] {
			centroidLow[k] = min(centroidLow[k], value)
			centroidHigh[k] = max(centroidHigh[k], value)
		}
	}

	s.b.nodes = append(s.b.nodes, gpuWideNode{b: bound, count: len(ids), left: -1, right: -1})

	leaf := func() int {
		s.b.nodes[index].tris = append([]uint32(nil), ids...)

		return index
	}
	if len(ids) == 1 {
		return leaf()
	}

	bestCost, axis, split := math.Inf(1), -1, -1

	if depth < 32 {
		for k := range 3 {
			extent := centroidHigh[k] - centroidLow[k]
			if extent <= 0 {
				continue
			}

			var bins [gpuWideSAHBins]gpuWideBin
			for i := range bins {
				bins[i].bounds = emptyGPUWideBox()
			}

			for _, id := range ids {
				i := gpuWideBinIndex(s.centers[id][k], centroidLow[k], extent)
				bins[i].bounds = mergeGPUWideBox(bins[i].bounds, s.boxes[id])
				bins[i].count++
			}

			var suffix [gpuWideSAHBins]gpuWideBin

			right := gpuWideBin{bounds: emptyGPUWideBox()}

			for i := gpuWideSAHBins - 1; i >= 0; i-- {
				if bins[i].count > 0 {
					right.bounds = mergeGPUWideBox(right.bounds, bins[i].bounds)
					right.count += bins[i].count
				}

				suffix[i] = right
			}

			left := gpuWideBin{bounds: emptyGPUWideBox()}

			for i := range gpuWideSAHBins - 1 {
				if bins[i].count > 0 {
					left.bounds = mergeGPUWideBox(left.bounds, bins[i].bounds)
					left.count += bins[i].count
				}

				if left.count == 0 || suffix[i+1].count == 0 {
					continue
				}

				cost := gpuWideArea(left.bounds)*float64(left.count) + gpuWideArea(suffix[i+1].bounds)*float64(suffix[i+1].count)
				if cost < bestCost {
					bestCost, axis, split = cost, k, i
				}
			}
		}
	}
	// Permit one/two/three-triangle leaves when splitting adds work; unlike
	// importing an existing leaf8 tree, evaluate SAH through every small range.
	if len(ids) <= 3 && (axis < 0 || gpuWideArea(bound)+bestCost >= gpuWideArea(bound)*float64(len(ids))) {
		return leaf()
	}

	middle := 0

	if axis >= 0 {
		extent := centroidHigh[axis] - centroidLow[axis]

		end := len(ids) - 1
		for middle <= end {
			if gpuWideBinIndex(s.centers[ids[middle]][axis], centroidLow[axis], extent) <= split {
				middle++
			} else {
				ids[middle], ids[end] = ids[end], ids[middle]
				end--
			}
		}
	}

	if middle <= 0 || middle >= len(ids) {
		axis = 0
		for k := 1; k < 3; k++ {
			if centroidHigh[k]-centroidLow[k] > centroidHigh[axis]-centroidLow[axis] {
				axis = k
			}
		}

		sort.Slice(ids, func(i, j int) bool {
			a, b := ids[i], ids[j]
			x, y := s.centers[a][axis], s.centers[b][axis]

			return x < y || x == y && a < b
		})
		middle = len(ids) / 2
	}

	left, right := s.build(ids[:middle], depth+1), s.build(ids[middle:], depth+1)
	s.b.nodes[index].left, s.b.nodes[index].right = left, right

	return index
}
