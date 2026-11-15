//nolint:mnd,varnamelen // Ray/sphere quadratics and plane intersections use explicit geometric equations.
package lightmapper

import "math"

type EmitterHit struct {
	Emitter  uint32
	Distance float32
	Normal   Vec3
	Found    bool
}

func emitterIntersection(ray Ray, e Emitter) (float32, Vec3, bool) {
	if e.Kind == Sphere {
		oc := Sub(ray.Origin, e.Position)
		a := Dot(ray.Direction, ray.Direction)
		b := Dot(oc, ray.Direction)
		c := Dot(oc, oc) - e.Radius*e.Radius

		d := b*b - a*c
		if d < 0 {
			return 0, Vec3{}, false
		}

		root := float32(math.Sqrt(float64(d)))

		distance := (-b - root) / a
		if distance < ray.Min {
			distance = (-b + root) / a
		}

		if distance < ray.Min || distance >= ray.Max {
			return 0, Vec3{}, false
		}

		return distance, Normalize(Sub(Add(ray.Origin, Scale(ray.Direction, distance)), e.Position)), true
	}

	if e.Kind != Rectangle && e.Kind != Disk {
		return 0, Vec3{}, false
	}

	denominator := Dot(ray.Direction, e.Direction)
	if float32(math.Abs(float64(denominator))) < 1e-12 {
		return 0, Vec3{}, false
	}

	distance := Dot(Sub(e.Position, ray.Origin), e.Direction) / denominator
	if distance < ray.Min || distance >= ray.Max {
		return 0, Vec3{}, false
	}

	local := Sub(Add(ray.Origin, Scale(ray.Direction, distance)), e.Position)
	if e.Kind == Disk {
		if Dot(local, local) > e.Radius*e.Radius {
			return 0, Vec3{}, false
		}
	} else {
		u, v := Dot(local, e.Tangent), Dot(local, Cross(e.Direction, e.Tangent))
		if float32(math.Abs(float64(u))) > e.Size[0]*.5 || float32(math.Abs(float64(v))) > e.Size[1]*.5 {
			return 0, Vec3{}, false
		}
	}

	return distance, e.Direction, true
}

// IntersectEmitter traverses the light hierarchy; analytic emitting geometry
// participates in continuation and shadow visibility without a source scan.
func (p *PreparedScene) IntersectEmitter(ray Ray, anyHit bool) EmitterHit {
	result := EmitterHit{Distance: ray.Max}
	if len(p.lightNodes) == 0 {
		return result
	}

	var stack [64]uint32

	size := 1
	for size > 0 {
		size--

		n := p.lightNodes[stack[size]]
		if !rayBox(ray, n.Min, n.Max, result.Distance) {
			continue
		}

		if n.Count == 0 {
			stack[size] = n.Left
			stack[size+1] = n.Right
			size += 2

			continue
		}

		index := p.lightOrder[n.First]

		distance, normal, found := emitterIntersection(ray, p.source.Emitters[index])
		if found && distance < result.Distance {
			result = EmitterHit{Emitter: index, Distance: distance, Normal: normal, Found: true}
			if anyHit {
				return result
			}
		}
	}

	return result
}

func (p *PreparedScene) Occluded(ray Ray) bool {
	return p.Intersect(ray, true).Found || p.IntersectEmitter(ray, true).Found
}
