//nolint:mnd,varnamelen // Emitter surface parameterizations and solid-angle Jacobians use explicit geometric equations.
package lightmapper

import "math"

type LightSample struct {
	Emitter             uint32
	Direction, Radiance Vec3
	Distance, PDF       float32
	Delta               bool
}

//nolint:exhaustive // Delta and infinite sources have no finite surface area.
func (p *PreparedScene) area(e Emitter) float32 {
	switch e.Kind {
	case Rectangle:
		return e.Size[0] * e.Size[1]
	case Disk:
		return lightPi * e.Radius * e.Radius
	case Sphere:
		return 4 * lightPi * e.Radius * e.Radius
	case EmissiveTriangle:
		t := p.source.Triangles[e.Triangle]

		return Length(Cross(Sub(t.Positions[1], t.Positions[0]), Sub(t.Positions[2], t.Positions[0]))) * .5
	}

	return 0
}

func spotProfile(e Emitter, direction Vec3) float32 {
	if e.Kind != Spot {
		return 1
	}

	c := Dot(e.Direction, Scale(direction, -1))
	if c >= e.InnerCos {
		return 1
	}

	if c <= e.OuterCos {
		return 0
	}

	t := (c - e.OuterCos) / (e.InnerCos - e.OuterCos)

	return t * t * (3 - 2*t)
}

func (p *PreparedScene) SampleEmitter(position Vec3, index uint32, u, v float32) LightSample {
	e := p.source.Emitters[index]

	result := LightSample{Emitter: index, Distance: math.MaxFloat32, PDF: 1, Delta: true}
	if e.Kind == EnvironmentSource {
		result.Direction, result.PDF = p.SampleEnvironment(u, v)
		result.Radiance = p.EnvironmentRadiance(result.Direction)
		result.Delta = false

		return result
	}

	if e.Kind == Directional {
		result.Direction = Scale(e.Direction, -1)
		result.Radiance = e.Intensity

		return result
	}

	if e.Kind == Sun {
		return sampleSun(e, index, u, v)
	}

	point, normal := p.emitterPosition(e, u, v)
	delta := Sub(point, position)

	distance := Length(delta)
	if distance <= 0 {
		return LightSample{Emitter: index}
	}

	result.Distance = distance

	result.Direction = Scale(delta, 1/distance)
	if e.Range > 0 && distance >= e.Range {
		return result
	}

	if e.Kind == Point || e.Kind == Spot {
		meters := distance * p.source.MetresPerUnit
		factor := float32(1) / (meters * meters)

		if e.Model == LegacyRangeV1 {
			f := max(0, 1-distance*distance/(e.Range*e.Range))
			factor = f * f
		}

		result.Radiance = Scale(e.Intensity, factor*spotProfile(e, result.Direction))

		return result
	}

	result.Delta = false

	cosine := Dot(normal, Scale(result.Direction, -1))
	if e.TwoSided {
		cosine = float32(math.Abs(float64(cosine)))
	}

	if cosine <= 0 {
		result.PDF = 0

		return result
	}

	result.PDF = distance * distance / (p.area(e) * cosine)
	result.Radiance = e.Intensity

	return result
}

func (*PreparedScene) sampleUniformSphere(u, v float32) (Vec3, float32) {
	y := 1 - 2*u
	r := float32(math.Sqrt(float64(max(0, 1-y*y))))
	a := 2 * lightPi * v

	return Vec3{r * float32(math.Cos(float64(a))), y, r * float32(math.Sin(float64(a)))}, 1 / (4 * lightPi)
}

// EmitterPDF is conditional solid-angle density, excluding light selection.
// distance/normal must refer to the first actually reached emitting surface.
func (p *PreparedScene) EmitterPDF(position Vec3, index uint32, direction Vec3, distance float32, normal Vec3) float32 {
	e := p.source.Emitters[index]
	if e.Kind == EnvironmentSource {
		return p.EnvironmentPDF(direction)
	}

	if e.Kind == Sun {
		return sunPDF(e, direction)
	}

	if e.Kind == Point || e.Kind == Spot || e.Kind == Directional {
		return 0
	}

	if e.Range > 0 && distance >= e.Range {
		return 0
	}

	c := Dot(normal, Scale(direction, -1))
	if e.TwoSided {
		c = float32(math.Abs(float64(c)))
	}

	if c <= 0 {
		return 0
	}

	return distance * distance / (p.area(e) * c)
}

//nolint:exhaustive // Infinite source kinds return before finite-surface position sampling.
func (p *PreparedScene) emitterPosition(e Emitter, u, v float32) (Vec3, Vec3) {
	var point, normal Vec3

	switch e.Kind {
	case Point, Spot:
		point = e.Position
	case Rectangle:
		point = Add(e.Position, Add(Scale(e.Tangent, (u-.5)*e.Size[0]), Scale(Cross(e.Direction, e.Tangent), (v-.5)*e.Size[1])))
		normal = e.Direction
	case Disk:
		r := e.Radius * float32(math.Sqrt(float64(u)))
		a := 2 * lightPi * v
		point = Add(
			e.Position,
			Add(Scale(e.Tangent, r*float32(math.Cos(float64(a)))), Scale(Cross(e.Direction, e.Tangent), r*float32(math.Sin(float64(a))))),
		)
		normal = e.Direction
	case Sphere:
		normal, _ = p.sampleUniformSphere(u, v)
		point = Add(e.Position, Scale(normal, e.Radius))
	case EmissiveTriangle:
		t := p.source.Triangles[e.Triangle]
		a := float32(math.Sqrt(float64(u)))
		point = Add(Scale(t.Positions[0], 1-a), Add(Scale(t.Positions[1], a*(1-v)), Scale(t.Positions[2], a*v)))
		normal = Normalize(Cross(Sub(t.Positions[1], t.Positions[0]), Sub(t.Positions[2], t.Positions[0])))
	}

	return point, normal
}
