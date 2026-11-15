//nolint:mnd,varnamelen // Cone solid angles use explicit trigonometric constants and conventional vector notation.
package lightmapper

import "math"

//nolint:exhaustive // Unsupported enum values are rejected during emitter admission; omitted kinds are omnidirectional.
func (p *PreparedScene) emitterCone(e Emitter) (Vec3, float32) {
	if e.TwoSided {
		return Vec3{}, -1
	}

	switch e.Kind {
	case Spot:
		return e.Direction, e.OuterCos
	case Rectangle, Disk:
		return e.Direction, 0
	case EmissiveTriangle:
		t := p.source.Triangles[e.Triangle]

		return Normalize(Cross(Sub(t.Positions[1], t.Positions[0]), Sub(t.Positions[2], t.Positions[0]))), 0
	}

	return Vec3{}, -1
}

func mergeLightCones(a, b LightNode) (Vec3, float32) {
	if a.Power == 0 {
		return b.Direction, b.ConeCos
	}

	if b.Power == 0 {
		return a.Direction, a.ConeCos
	}

	if a.ConeCos <= -1 || b.ConeCos <= -1 {
		return Vec3{}, -1
	}

	axis := Normalize(Add(Scale(a.Direction, a.Power), Scale(b.Direction, b.Power)))
	if axis == (Vec3{}) {
		return Vec3{}, -1
	}

	halfA := math.Acos(float64(max(-1, min(1, Dot(axis, a.Direction))))) + math.Acos(float64(max(-1, min(1, a.ConeCos))))
	halfB := math.Acos(float64(max(-1, min(1, Dot(axis, b.Direction))))) + math.Acos(float64(max(-1, min(1, b.ConeCos))))

	half := max(halfA, halfB)
	if half >= math.Pi {
		return Vec3{}, -1
	}

	return axis, float32(math.Cos(half))
}

func lightConeImportance(position Vec3, n LightNode) float32 {
	if n.ConeCos <= -1 {
		return 1
	}

	center := Add(Scale(n.Min, .5), Scale(n.Max, .5))
	delta := Sub(position, center)
	distance := Length(delta)

	radius := Length(Sub(n.Max, n.Min)) * .5
	if !validVec(delta) || !finite(distance) || !finite(radius) {
		return 1
	}

	if distance <= radius {
		return 1
	}

	angle := math.Acos(float64(max(-1, min(1, Dot(n.Direction, Scale(delta, 1/distance))))))
	half := math.Acos(float64(max(-1, min(1, n.ConeCos)))) + math.Asin(float64(max(0, min(1, radius/distance))))

	return max(.03125, float32(math.Cos(max(0, angle-half))))
}
