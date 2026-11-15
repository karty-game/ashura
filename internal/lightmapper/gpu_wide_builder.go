//go:build !js

//nolint:varnamelen,mnd,gocognit,funlen // Bounded SAH dynamic programming and quantized octant assignment keep conventional geometric indices together.
package lightmapper

import (
	"math"
	"sort"
)

type gpuWideBox struct{ lo, hi [3]float32 }
type gpuWideNode struct {
	b           gpuWideBox
	left, right int
	tris        []uint32
	count       int
	cost        [9]float64
	split       [9]int
	wideK       int
}
type gpuWideBuilder struct {
	scene    *PreparedScene
	nodes    []gpuWideNode
	oldOrder []uint32
	packed   []uint32
	order    []uint32
	depth    int
	balanced bool
	linear   bool
	err      error
}

func (b *gpuWideBuilder) vertex(t uint32, c, a int) float32 {
	return b.scene.source.Triangles[t].Positions[c][a]
}
func emptyGPUWideBox() gpuWideBox {
	return gpuWideBox{
		[3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		[3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}
}
func gpuWideArea(v gpuWideBox) float64 {
	x := float64(v.hi[0]) - float64(v.lo[0])
	y := float64(v.hi[1]) - float64(v.lo[1])
	z := float64(v.hi[2]) - float64(v.lo[2])

	return x*y + x*z + y*z
}
func (b *gpuWideBuilder) bounds(tris []uint32) gpuWideBox {
	v := emptyGPUWideBox()

	for _, t := range tris {
		for c := range 3 {
			for a := range 3 {
				x := b.vertex(t, c, a)
				v.lo[a] = min(v.lo[a], x)
				v.hi[a] = max(v.hi[a], x)
			}
		}
	}

	return v
}
func (b *gpuWideBuilder) micro(tris []uint32) int {
	v := b.bounds(tris)
	id := len(b.nodes)

	b.nodes = append(b.nodes, gpuWideNode{b: v, count: len(tris), left: -1, right: -1})
	if len(tris) <= 3 {
		b.nodes[id].tris = append([]uint32{}, tris...)

		return id
	}

	axis := 0
	for a := 1; a < 3; a++ {
		if float64(v.hi[a])-float64(v.lo[a]) > float64(v.hi[axis])-float64(v.lo[axis]) {
			axis = a
		}
	}

	copyIDs := append([]uint32{}, tris...)
	centroid := func(t uint32) float64 {
		return float64(b.vertex(t, 0, axis)) + float64(b.vertex(t, 1, axis)) + float64(b.vertex(t, 2, axis))
	}

	sort.SliceStable(copyIDs, func(i, j int) bool {
		x, y := centroid(copyIDs[i]), centroid(copyIDs[j])
		if x == y {
			return copyIDs[i] < copyIDs[j]
		}

		return x < y
	})
	left := b.micro(copyIDs[:len(tris)/2])
	right := b.micro(copyIDs[len(tris)/2:])
	b.nodes[id].left = left
	b.nodes[id].right = right

	return id
}
func (b *gpuWideBuilder) dp(id int) {
	n := &b.nodes[id]
	for i := range n.cost {
		n.cost[i] = math.Inf(1)
	}

	if len(n.tris) > 0 {
		n.cost[1] = gpuWideArea(n.b) * float64(len(n.tris))

		return
	}

	l, r := n.left, n.right
	b.dp(l)
	b.dp(r)
	n = &b.nodes[id]
	best := math.Inf(1)

	for k := 2; k <= 8; k++ {
		for i := 1; i < k; i++ {
			v := b.nodes[l].cost[i] + b.nodes[r].cost[k-i]
			if v < n.cost[k] {
				n.cost[k] = v
				n.split[k] = i
			}
		}

		if n.cost[k] < best {
			best = n.cost[k]
			n.wideK = k
		}
	}

	n.cost[1] = gpuWideArea(n.b) + best
}
func (b *gpuWideBuilder) frontier(id, k int, out *[]int) {
	if k == 1 {
		*out = append(*out, id)

		return
	}

	n := b.nodes[id]
	b.frontier(n.left, n.split[k], out)
	b.frontier(n.right, k-n.split[k], out)
}
func gpuWideDecodedBound(origin float32, q int, e int, upper bool) float64 {
	if q == 0 {
		return float64(origin)
	}

	v := float64(origin) + math.Ldexp(float64(q), e)

	f := float32(v)
	if upper {
		return float64(math.Nextafter32(f, float32(math.Inf(1))))
	}

	return float64(math.Nextafter32(f, float32(math.Inf(-1))))
}
func gpuWideQuantize(origin float32, lo, hi float32, e int) (int, int, bool) {
	step := math.Ldexp(1, e)
	l := int(math.Floor((float64(lo) - float64(origin)) / step))
	h := int(math.Ceil((float64(hi) - float64(origin)) / step))
	l = max(0, min(l, 255))
	h = max(0, h)

	for l > 0 && gpuWideDecodedBound(origin, l, e, true) > float64(lo) {
		l--
	}

	for h <= 255 && gpuWideDecodedBound(origin, h, e, false) < float64(hi) {
		h++
	}

	return l, h, h <= 255
}

//nolint:gosec // G602: each selected slot is one of eight unused entries and each child index belongs to the bounded frontier.
func (b *gpuWideBuilder) assignSlots(n gpuWideNode, children []int) [8]int {
	// Deterministic greedy child assignment to ray-direction octants.
	var slots [8]int
	for i := range slots {
		slots[i] = -1
	}

	used := make([]bool, len(children))
	for range children {
		best := math.Inf(1)
		bs, bc := -1, -1

		for s := range 8 {
			if slots[s] >= 0 {
				continue
			}

			for c, child := range children {
				if used[c] {
					continue
				}

				var cost float64

				for a := range 3 {
					d := (float64(b.nodes[child].b.lo[a]) + float64(b.nodes[child].b.hi[a]) - float64(n.b.lo[a]) - float64(n.b.hi[a])) * .5
					if s&(1<<(2-a)) != 0 {
						d = -d
					}

					cost += d
				}

				if cost < best {
					best = cost
					bs = s
					bc = c
				}
			}
		}

		if bs < 0 || bc < 0 {
			b.err = ErrInvalid

			return slots
		}

		slots[bs] = children[bc]
		used[bc] = true
	}

	return slots
}

type gpuWideQuantization struct {
	exponents [3]int
	low, high [3][8]int
}

func (b *gpuWideBuilder) quantizeNode(n gpuWideNode, slots [8]int, at int) gpuWideQuantization {
	var (
		exp      [3]int
		qlo, qhi [3][8]int
	)

	for a := range 3 {
		// Quantized traversal decodes q*scale+origin on float32 hardware.
		// Do not require a mathematically finite expression to survive an
		// overflowing product or a flushed subnormal origin.
		if n.b.lo[a] != 0 && math.Abs(float64(n.b.lo[a])) < 0x1p-126 {
			b.err = ErrBounds

			return gpuWideQuantization{}
		}

		extent := float64(n.b.hi[a]) - float64(n.b.lo[a])

		e := -126
		if extent > 0 {
			e = max(-126, int(math.Ceil(math.Log2(extent/255))))
		}

		for {
			if e > 126 {
				b.err = ErrBounds

				return gpuWideQuantization{}
			}

			ok := true

			for slot, child := range slots {
				if child < 0 {
					continue
				}

				l, h, valid := gpuWideQuantize(n.b.lo[a], b.nodes[child].b.lo[a], b.nodes[child].b.hi[a], e)
				qlo[a][slot] = l
				qhi[a][slot] = h
				ok = ok && valid
			}

			if ok {
				break
			}

			e++
		}

		exp[a] = e

		for slot, child := range slots {
			if child < 0 {
				continue
			}

			product := math.Ldexp(float64(qhi[a][slot]), e)

			decoded := float32(float64(n.b.lo[a]) + product)
			if product > math.MaxFloat32 || math.IsInf(float64(decoded), 0) {
				b.err = ErrBounds

				return gpuWideQuantization{}
			}
		}

		b.packed[at+a] = math.Float32bits(n.b.lo[a])
	}

	return gpuWideQuantization{exponents: exp, low: qlo, high: qhi}
}

func (b *gpuWideBuilder) emit(id, index, depth int) {
	b.depth = max(b.depth, depth)
	n := b.nodes[id]

	children := []int{}

	switch {
	case len(n.tris) > 0:
		children = append(children, id)
	case b.balanced:
		children = b.balancedFrontier(id)
	default:
		b.frontier(id, n.wideK, &children)
	}

	slots := b.assignSlots(n, children)
	if b.err != nil {
		return
	}

	at := index * gpuNodeWords

	quantized := b.quantizeNode(n, slots, at)
	if b.err != nil {
		return
	}

	exp, qlo, qhi := quantized.exponents, quantized.low, quantized.high
	next := len(b.packed) / 20
	pending := [][2]int{}
	triBase := len(b.order)

	for slot, child := range slots {
		if child < 0 {
			continue
		}

		cn := b.nodes[child]

		var meta uint32
		if len(cn.tris) > 0 {
			meta = (uint32((1<<len(cn.tris))-1) << 5) | gpuOffsetWord(len(b.order)-triBase)
			b.order = append(b.order, cn.tris...)
		} else {
			if len(pending) == 0 {
				b.packed[at+4] = gpuOffsetWord(next)
			}

			pending = append(pending, [2]int{child, next})
			b.packed = append(b.packed, make([]uint32, 20)...)
			next++
			meta = (1 << 5) | uint32(24+slot)
			b.packed[at+3] |= 1 << uint(24+slot)
		}

		b.packed[at+6+slot/4] |= meta << uint((slot%4)*8)
		for a := range 3 {
			b.packed[at+8+a*2+slot/4] |= gpuOffsetWord(qlo[a][slot]) << uint((slot%4)*8)

			b.packed[at+14+a*2+slot/4] |= gpuOffsetWord(qhi[a][slot]) << uint((slot%4)*8)
			if gpuWideDecodedBound(n.b.lo[a], qlo[a][slot], exp[a], true) > float64(cn.b.lo[a]) ||
				gpuWideDecodedBound(n.b.lo[a], qhi[a][slot], exp[a], false) < float64(cn.b.hi[a]) {
				b.err = ErrInvalid

				return
			}
		}
	}

	b.packed[at+3] |= gpuOffsetWord(exp[0]+127) | gpuOffsetWord(exp[1]+127)<<8 | gpuOffsetWord(exp[2]+127)<<16

	b.packed[at+5] = gpuOffsetWord(triBase * 4)
	for _, p := range pending {
		if b.err != nil {
			return
		}

		b.emit(p[0], p[1], depth+1)
	}
}
