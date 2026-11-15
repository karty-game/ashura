//nolint:mnd,varnamelen // Portable BVH traversal and barycentric intersections use fixed stack bounds and vector algebra.
package lightmapper

import (
	"math"
	"sort"
)

type Ray struct {
	Origin, Direction Vec3
	Min, Max          float32
}
type Hit struct {
	Triangle    uint32
	Distance    float32
	Barycentric Vec3
	Found       bool
}

//nolint:gosec,nestif //  Scene admission bounds triangle/node indices below2^21 before wire conversion. The bounded SAH builder handles one complete leaf or deterministic median fallback before returning.
func (p *PreparedScene) buildGeometry() {
	p.order = make([]uint32, len(p.source.Triangles))
	for i := range p.order {
		p.order[i] = uint32(i)
	}

	if len(p.order) == 0 {
		return
	}

	var build func(int, int, int) uint32

	build = func(first, end, depth int) uint32 {
		index := uint32(len(p.nodes))
		p.nodes = append(p.nodes, BVHNode{})
		lo, hi := Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}

		for _, i := range p.order[first:end] {
			for _, v := range p.source.Triangles[i].Positions {
				lo = minVec(lo, v)
				hi = maxVec(hi, v)
			}
		}

		n := BVHNode{Min: lo, Max: hi}
		if end-first <= 8 {
			n.First, n.Count = uint32(first), uint32(end-first)
		} else {
			middle := -1
			if depth < 32 {
				middle = p.sahPartition(first, end)
			}

			if middle <= first || middle >= end {
				d := Sub(hi, lo)

				axis := 0
				if d[1] > d[axis] {
					axis = 1
				}

				if d[2] > d[axis] {
					axis = 2
				}

				sort.Slice(p.order[first:end], func(a, b int) bool {
					ta, tb := p.source.Triangles[p.order[first+a]], p.source.Triangles[p.order[first+b]]
					ca := ta.Positions[0][axis] + ta.Positions[1][axis] + ta.Positions[2][axis]

					cb := tb.Positions[0][axis] + tb.Positions[1][axis] + tb.Positions[2][axis]
					if ca == cb {
						return ta.ID < tb.ID
					}

					return ca < cb
				})
				middle = (first + end) / 2
			}

			n.Left = build(first, middle, depth+1)
			n.Right = build(middle, end, depth+1)
		}

		p.nodes[index] = n

		return index
	}
	build(0, len(p.order), 0)
}

func rayBox(ray Ray, lo, hi Vec3, maximum float32) bool {
	near, far := ray.Min, maximum
	for a := range 3 {
		if float32(math.Abs(float64(ray.Direction[a]))) < 1e-20 {
			if ray.Origin[a] < lo[a] || ray.Origin[a] > hi[a] {
				return false
			}

			continue
		}

		v1, v2 := (lo[a]-ray.Origin[a])/ray.Direction[a], (hi[a]-ray.Origin[a])/ray.Direction[a]
		near = max(near, min(v1, v2))

		far = min(far, max(v1, v2))
		if near > far {
			return false
		}
	}

	return true
}

func rayTriangle(ray Ray, t Triangle) (float32, Vec3, bool) {
	e1, e2 := Sub(t.Positions[1], t.Positions[0]), Sub(t.Positions[2], t.Positions[0])
	q := Cross(ray.Direction, e2)

	d := Dot(e1, q)
	if float32(math.Abs(float64(d))) <= 1e-7*Length(Cross(e1, e2))*Length(ray.Direction) {
		return 0, Vec3{}, false
	}

	s := Sub(ray.Origin, t.Positions[0])

	u := Dot(s, q) / d
	if u < 0 || u > 1 {
		return 0, Vec3{}, false
	}

	r := Cross(s, e1)

	v := Dot(ray.Direction, r) / d
	if v < 0 || u+v > 1 {
		return 0, Vec3{}, false
	}

	distance := Dot(e2, r) / d

	return distance, Vec3{1 - u - v, u, v}, distance >= ray.Min && distance < ray.Max
}

func (p *PreparedScene) Intersect(ray Ray, anyHit bool) Hit {
	result := Hit{Distance: ray.Max}
	if len(p.nodes) == 0 {
		return result
	}

	var stack [64]uint32

	size := 1
	for size > 0 {
		size--

		node := p.nodes[stack[size]]
		if !rayBox(ray, node.Min, node.Max, result.Distance) {
			continue
		}

		if node.Count == 0 {
			stack[size] = node.Left
			stack[size+1] = node.Right
			size += 2

			continue
		}

		for _, i := range p.order[node.First : node.First+node.Count] {
			distance, bary, found := rayTriangle(ray, p.source.Triangles[i])
			if found && distance < result.Distance {
				result = Hit{Triangle: i, Distance: distance, Barycentric: bary, Found: true}
				if anyHit {
					return result
				}
			}
		}
	}

	return result
}

func (p *PreparedScene) Surface(hit Hit) (position, geometric, normal Vec3, uv Vec2) {
	t := p.source.Triangles[hit.Triangle]
	for i := range 3 {
		position = Add(position, Scale(t.Positions[i], hit.Barycentric[i]))

		normal = Add(normal, Scale(t.Normals[i], hit.Barycentric[i]))
		for a := range 2 {
			uv[a] += t.UV[i][a] * hit.Barycentric[i]
		}
	}

	geometric = Normalize(Cross(Sub(t.Positions[1], t.Positions[0]), Sub(t.Positions[2], t.Positions[0])))
	normal = Normalize(normal)

	return
}
