//nolint:mnd,varnamelen // Convex coplanar overlap uses explicit projection and separation equations.
package lightmapper

import "math"

type overlapPoint [2]float64

func overlapDot(a, b overlapPoint) float64      { return a[0]*b[0] + a[1]*b[1] }
func overlapSub(a, b overlapPoint) overlapPoint { return overlapPoint{a[0] - b[0], a[1] - b[1]} }
func overlapCoordinates(v, origin, t, b Vec3) overlapPoint {
	return overlapPoint{
		float64(v[0]-origin[0])*float64(t[0]) + float64(v[1]-origin[1])*float64(t[1]) + float64(v[2]-origin[2])*float64(t[2]),
		float64(v[0]-origin[0])*float64(b[0]) + float64(v[1]-origin[1])*float64(b[1]) + float64(v[2]-origin[2])*float64(b[2]),
	}
}

func overlappingSurfaces(a, b overlapSurface, meters float32) bool {
	scale := max(float64(1/meters), float64(a.radius), float64(b.radius))
	for _, polygon := range [][]Vec3{a.polygon, b.polygon} {
		for _, v := range polygon {
			scale = max(scale, float64(Length(Sub(v, a.center))))
		}
	}

	epsilon := scale * 1e-10
	// Plane coincidence allows float32 normal/vertex reconstruction error;
	// positive-area separation is evaluated in the projected float64 plane.
	planeTolerance := scale * 2e-6

	if a.sphere || b.sphere {
		return a.sphere && b.sphere && float64(Length(Sub(a.center, b.center))) <= epsilon &&
			math.Abs(float64(a.radius)-float64(b.radius)) <= epsilon
	}

	if float64(Length(Cross(a.normal, b.normal))) > 1e-6 {
		return false
	}

	if math.Abs(float64(Dot(a.normal, Sub(b.center, a.center)))) > planeTolerance {
		return false
	}

	for _, v := range b.polygon {
		if math.Abs(float64(Dot(a.normal, Sub(v, a.center)))) > planeTolerance {
			return false
		}
	}

	if b.radius > 0 && float64(Length(Cross(a.normal, b.normal)))*float64(b.radius) > planeTolerance {
		return false
	}

	tangent, bitangent := tangentFrame(a.normal)
	project := func(shape overlapSurface) []overlapPoint {
		points := make([]overlapPoint, len(shape.polygon))
		for i, v := range shape.polygon {
			points[i] = overlapCoordinates(v, a.center, tangent, bitangent)
		}

		return points
	}
	pa, pb := project(a), project(b)

	ca, cb := overlapCoordinates(a.center, a.center, tangent, bitangent), overlapCoordinates(b.center, a.center, tangent, bitangent)
	if a.radius > 0 && b.radius > 0 {
		return math.Hypot(ca[0]-cb[0], ca[1]-cb[1]) < float64(a.radius+b.radius)-epsilon
	}

	if a.radius > 0 {
		return overlapCirclePolygon(ca, float64(a.radius), pb, epsilon)
	}

	if b.radius > 0 {
		return overlapCirclePolygon(cb, float64(b.radius), pa, epsilon)
	}

	return overlapPolygons(pa, pb, epsilon)
}

func overlapPolygons(a, b []overlapPoint, epsilon float64) bool {
	for _, polygon := range [][]overlapPoint{a, b} {
		for i, v := range polygon {
			edge := overlapSub(polygon[(i+1)%len(polygon)], v)

			length := math.Hypot(edge[0], edge[1])
			if length == 0 {
				continue
			}

			axis := overlapPoint{-edge[1] / length, edge[0] / length}
			loA, hiA := math.Inf(1), math.Inf(-1)
			loB, hiB := math.Inf(1), math.Inf(-1)

			for _, p := range a {
				x := overlapDot(p, axis)
				loA = min(loA, x)
				hiA = max(hiA, x)
			}

			for _, p := range b {
				x := overlapDot(p, axis)
				loB = min(loB, x)
				hiB = max(hiB, x)
			}

			if min(hiA, hiB)-max(loA, loB) <= epsilon {
				return false
			}
		}
	}

	return len(a) >= 3 && len(b) >= 3
}

func overlapCirclePolygon(center overlapPoint, radius float64, polygon []overlapPoint, epsilon float64) bool {
	inside := true
	orientation := float64(0)

	for i, v := range polygon {
		next := polygon[(i+1)%len(polygon)]
		edge := overlapSub(next, v)
		delta := overlapSub(center, v)

		cross := edge[0]*delta[1] - edge[1]*delta[0]
		if orientation == 0 && math.Abs(cross) > epsilon {
			orientation = math.Copysign(1, cross)
		} else if cross*orientation < -epsilon {
			inside = false
		}

		denominator := overlapDot(edge, edge)
		if denominator == 0 {
			continue
		}

		fraction := max(0, min(1, overlapDot(delta, edge)/denominator))

		closest := overlapPoint{v[0] + fraction*edge[0], v[1] + fraction*edge[1]}
		if math.Hypot(center[0]-closest[0], center[1]-closest[1]) < radius-epsilon {
			return true
		}
	}

	return inside && len(polygon) >= 3
}
