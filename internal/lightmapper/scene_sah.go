//nolint:varnamelen // SAH bin costs use conventional geometric bounds notation.
package lightmapper

import "math"

const sceneSAHBins = 12

type sceneSAHBox struct {
	lo, hi Vec3
	count  int
}

func emptySAHBox() sceneSAHBox {
	return sceneSAHBox{
		lo: Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		hi: Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
}
func (b *sceneSAHBox) add(t Triangle) {
	for _, v := range t.Positions {
		b.lo = minVec(b.lo, v)
		b.hi = maxVec(b.hi, v)
	}

	b.count++
}
func (b *sceneSAHBox) merge(other sceneSAHBox) {
	if other.count == 0 {
		return
	}

	b.lo = minVec(b.lo, other.lo)
	b.hi = maxVec(b.hi, other.hi)
	b.count += other.count
}
func (b *sceneSAHBox) cost() float64 {
	if b.count == 0 {
		return 0
	}

	x, y, z := float64(b.hi[0])-float64(b.lo[0]), float64(b.hi[1])-float64(b.lo[1]), float64(b.hi[2])-float64(b.lo[2])

	return (x*y + y*z + x*z) * float64(b.count)
}

//nolint:mnd // A triangle centroid is the mean of its three corner coordinates.
func sceneCentroid(t Triangle, axis int) float64 {
	return (float64(t.Positions[0][axis]) + float64(t.Positions[1][axis]) + float64(t.Positions[2][axis])) / 3
}
func sceneSAHBin(value, lo, extent float64) int {
	return min(sceneSAHBins-1, max(0, int((value-lo)/extent*sceneSAHBins)))
}

// Twelve-bin SAH splits reduce software traversal work without changing triangle
// identities or visibility. A median fallback after32 levels keeps the admitted
// million-triangle scene below the64-entry shader/reference traversal stack.
func (p *PreparedScene) sahPartition(first, end int) int {
	bestCost := math.Inf(1)
	bestAxis, bestBin := -1, -1
	bestLo, bestExtent := float64(0), float64(0)

	for axis := range 3 {
		lo, hi := math.Inf(1), math.Inf(-1)

		for _, index := range p.order[first:end] {
			v := sceneCentroid(p.source.Triangles[index], axis)
			lo = min(lo, v)
			hi = max(hi, v)
		}

		if hi <= lo {
			continue
		}

		extent := hi - lo

		var bins [sceneSAHBins]sceneSAHBox
		for i := range bins {
			bins[i] = emptySAHBox()
		}

		for _, index := range p.order[first:end] {
			t := p.source.Triangles[index]
			at := sceneSAHBin(sceneCentroid(t, axis), lo, extent)
			bins[at].add(t)
		}

		var right [sceneSAHBins]sceneSAHBox

		total := emptySAHBox()
		for i := sceneSAHBins - 1; i >= 0; i-- {
			total.merge(bins[i])
			right[i] = total
		}

		left := emptySAHBox()
		for i := range sceneSAHBins - 1 {
			left.merge(bins[i])

			if left.count == 0 || right[i+1].count == 0 {
				continue
			}

			cost := left.cost() + right[i+1].cost()
			if cost < bestCost {
				bestCost = cost
				bestAxis, bestBin = axis, i
				bestLo, bestExtent = lo, extent
			}
		}
	}

	if bestAxis < 0 {
		return -1
	}
	// In-place deterministic partition. No per-node temporary triangle array is
	// retained alongside the source geometry and the uploaded packed scene.
	left, right := first, end-1
	for left <= right {
		t := p.source.Triangles[p.order[left]]
		if sceneSAHBin(sceneCentroid(t, bestAxis), bestLo, bestExtent) <= bestBin {
			left++
		} else {
			p.order[left], p.order[right] = p.order[right], p.order[left]
			right--
		}
	}

	return left
}
