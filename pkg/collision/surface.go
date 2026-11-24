package collision

import "math"

type surface struct {
	a, b, c, normal vector
}

func (face surface) ray(start, delta vector, limit float64) (contact, bool) {
	approach := delta.dot(face.normal)
	if approach == 0 {
		return contact{}, false
	}

	fraction := face.a.sub(start).dot(face.normal) / approach
	if fraction < 0 || fraction > limit {
		return contact{}, false
	}

	point := start.add(delta.mul(fraction))
	if !face.contains(point) {
		return contact{}, false
	}

	normal := face.normal
	if approach > 0 {
		normal = normal.mul(-1)
	}

	return contact{point: point, normal: normal, fraction: fraction}, true
}

func (face surface) contains(point vector) bool {
	return insideEdge(face.a, face.b, point, face.normal) &&
		insideEdge(face.b, face.c, point, face.normal) &&
		insideEdge(face.c, face.a, point, face.normal)
}

func insideEdge(a, b, point, normal vector) bool {
	const roundoff = 1e-12

	edge, offset := b.sub(a), point.sub(a)
	side := edge.cross(offset).dot(normal)

	return side >= 0 || side >= -roundoff*math.Sqrt(edge.dot(edge)*offset.dot(offset))
}

func (query sweep) capsuleTriangle(face surface, limit float64) (contact, bool) {
	// The triangle is a subset of its plane. A capsule staying on the
	// outside of that plane can slide across coplanar triangle seams.
	distance := query.start.sub(face.a).dot(face.normal)
	support := query.radius + query.half*math.Abs(face.normal[1])

	normalSpeed := query.delta.dot(face.normal) * math.Copysign(1, distance)
	if math.Abs(distance) >= support-query.tolerance && normalSpeed >= -1e-12*query.speed {
		return contact{}, false
	}

	bottom := query.start.sub(vector{0, query.half, 0})
	axis := vector{0, query.half + query.half, 0}
	fraction := 0.0

	for range castIterations {
		point, axisPoint := face.closestToSegment(bottom.add(query.delta.mul(fraction)), axis)
		separation := axisPoint.sub(point)
		distance := math.Sqrt(separation.dot(separation))

		normal := face.normal
		if distance > 0 {
			normal = separation.mul(1 / distance)
		} else if query.delta.dot(normal) > 0 {
			normal = normal.mul(-1)
		}

		approach := -query.delta.dot(normal)
		gap := distance - query.radius

		if approach <= 0 {
			return contact{}, false
		}

		if gap <= query.tolerance {
			return contact{point: point, normal: normal, fraction: fraction}, true
		}

		// The closest pair supplies a separating plane. Contact cannot
		// occur before the moving capsule reaches that plane.
		fraction += gap / approach
		if fraction > limit {
			return contact{}, false
		}
	}

	return contact{}, false
}

// A closest pair lies at a segment/face intersection, at an endpoint projected
// onto the face, or between the segment and one of the triangle edges.
func (face surface) closestToSegment(start, delta vector) (vector, vector) {
	if hit, ok := face.ray(start, delta, 1); ok {
		return hit.point, hit.point
	}

	end := start.add(delta)
	best := math.Inf(1)

	var point, axisPoint vector

	for _, endpoint := range [2]vector{start, end} {
		height := endpoint.sub(face.a).dot(face.normal)

		projected := endpoint.sub(face.normal.mul(height))
		if height*height < best && face.contains(projected) {
			point, axisPoint, best = projected, endpoint, height*height
		}
	}

	vertices := [3]vector{face.a, face.b, face.c}
	for index, edgeStart := range vertices {
		edge := vertices[(index+1)%len(vertices)].sub(edgeStart)
		a, b := closestSegments(start, delta, edgeStart, edge)

		separation := a.sub(b)
		if distance := separation.dot(separation); distance < best {
			point, axisPoint, best = b, a, distance
		}
	}

	return point, axisPoint
}

func closestSegments(startA, alongA, startB, alongB vector) (vector, vector) {
	offset := startA.sub(startB)
	lengthA, lengthB := alongA.dot(alongA), alongB.dot(alongB)

	bOffset, coupling := alongB.dot(offset), alongA.dot(alongB)
	if lengthA == 0 {
		return startA, startB.add(alongB.mul(unitInterval(bOffset / lengthB)))
	}

	var fractionA float64

	aOffset := alongA.dot(offset)

	cross := alongA.cross(alongB)
	if determinant := cross.dot(cross); determinant > 0 {
		fractionA = unitInterval((coupling*bOffset - aOffset*lengthB) / determinant)
	}

	fractionB := (coupling*fractionA + bOffset) / lengthB
	if fractionB < 0 || fractionB > 1 {
		fractionB = unitInterval(fractionB)
		fractionA = unitInterval((fractionB*coupling - aOffset) / lengthA)
	}

	return startA.add(alongA.mul(fractionA)), startB.add(alongB.mul(fractionB))
}
