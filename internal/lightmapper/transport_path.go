package lightmapper

import "math"

//nolint:gocognit,cyclop,funlen //  This reference retains emission, continuation and roulette in their auditable path order. The scalar oracle retains emission, continuation and roulette in exact path order.
func (p *PreparedScene) traceReference(
	position, geometric, direction Vec3,
	pdf float32,
	receiverID uint64,
	sample uint32,
	options Options,
) referenceContribution {
	var result referenceContribution

	throughput := Vec3{1, 1, 1}
	origin := p.rayOffset(position, geometric)
	previous := position
	rng := func(bounce, dim uint32) float32 {
		return SampleDimension(receiverID, options.Seed, sample, bounce, dim)
	}

	for depth := uint32(0); depth <= options.Bounces; depth++ {
		ray := Ray{Origin: origin, Direction: direction, Min: p.rayMinimum(), Max: math.MaxFloat32}
		hit := p.Intersect(ray, false)

		area := p.IntersectEmitter(ray, false)
		if area.Found && (!hit.Found || area.Distance < hit.Distance) {
			emitter := p.source.Emitters[area.Emitter]

			cosine := Dot(area.Normal, Scale(direction, -1))
			if (emitter.TwoSided || cosine > 0) && (emitter.Range == 0 || area.Distance < emitter.Range) {
				weight := p.continuationMIS(previous, direction, pdf, area.Emitter, area.Distance, area.Normal, options)
				value := Scale(Mul(throughput, emitter.Intensity), weight)
				result.addRadiance(value, depth, false)
			}

			break
		}

		if !hit.Found {
			if p.environmentEmitter >= 0 {
				weight := p.continuationMIS(previous, direction, pdf, uint32(p.environmentEmitter), math.MaxFloat32, Vec3{}, options)
				result.addRadiance(Scale(Mul(throughput, p.EnvironmentRadiance(direction)), weight), depth, true)
			}

			break
		}

		point, geometricNormal, normal, surfaceUV := p.Surface(hit)
		material := p.source.Materials[p.source.Triangles[hit.Triangle].Material]

		front := Dot(geometricNormal, direction) < 0
		if !front {
			geometricNormal = Scale(geometricNormal, -1)
			normal = Scale(normal, -1)
		}

		if index := p.triangleEmitters[hit.Triangle]; index >= 0 {
			emitter := p.source.Emitters[index]
			if (front || emitter.TwoSided) && (emitter.Range == 0 || hit.Distance < emitter.Range) {
				weight := p.continuationMIS(previous, direction, pdf, uint32(index), hit.Distance, geometricNormal, options)
				result.addRadiance(Scale(Mul(throughput, emitter.Intensity), weight), depth, false)
			}
		}

		if !front && !material.DoubleSided {
			break
		}

		if depth == options.Bounces {
			break
		}

		reflectance := p.Reflectance(p.source.Triangles[hit.Triangle].Material, surfaceUV)
		nee := p.directSecondary(point, geometricNormal, normal, depth+1, options, rng)
		result.reflected = Add(result.reflected, Mul(throughput, Mul(reflectance, nee)))

		throughput = Mul(throughput, reflectance)
		if maxComponent(throughput) <= 0 {
			break
		}

		if options.RouletteDepth > 0 && depth+1 >= options.RouletteDepth {
			survival := max(.05, min(.95, maxComponent(throughput)))
			if rng(depth+1, 8) >= survival {
				break
			}

			throughput = Scale(throughput, 1/survival)
		}

		direction = sampleHemisphere(normal, rng(depth+1, 6), rng(depth+1, 7), true)
		if Dot(direction, geometricNormal) <= 0 {
			break
		}

		pdf = max(0, Dot(normal, direction)) / lightPi
		previous = point
		origin = p.rayOffset(point, geometricNormal)
	}

	return result
}

//nolint:mnd // The endpoint allowance stops shadow traversal before the sampled emitter surface.
func (p *PreparedScene) directSecondary(
	position, geometricNormal, normal Vec3,
	bounce uint32,
	options Options,
	rng func(uint32, uint32) float32,
) Vec3 {
	var result Vec3

	visit := func(index uint32, mass float32) {
		light := p.SampleEmitter(position, index, rng(bounce, 4), rng(bounce, 5))

		cosine := max(0, Dot(normal, light.Direction))
		if light.PDF <= 0 || mass <= 0 || cosine <= 0 || Dot(geometricNormal, light.Direction) <= 0 || maxComponent(light.Radiance) <= 0 {
			return
		}

		if p.Occluded(
			Ray{
				Origin:    p.rayOffset(position, geometricNormal),
				Direction: light.Direction,
				Min:       p.rayMinimum(),
				Max:       light.Distance * (1 - 2e-5),
			},
		) {
			return
		}

		weight := float32(1)
		if !light.Delta && p.source.Emitters[index].Kind != Sun {
			weight = misWeight(mass*light.PDF, cosine/lightPi)
		}

		result = Add(result, Scale(light.Radiance, cosine/lightPi*weight/(mass*light.PDF)))
	}

	if options.ExactLights {
		for i := range p.source.Emitters {
			visit(uint32(i), 1)
		}
	} else {
		index, mass := p.SampleLight(position, rng(bounce, 3))
		if mass > 0 {
			visit(index, mass)
		}
	}

	return result
}

func (p *PreparedScene) continuationMIS(
	previous, direction Vec3,
	pdf float32,
	emitter uint32,
	distance float32,
	normal Vec3,
	options Options,
) float32 {
	mass := p.LightPMF(previous, emitter)
	if options.ExactLights {
		mass = 1
	}

	return misWeight(pdf, mass*p.EmitterPDF(previous, emitter, direction, distance, normal))
}
func (r *referenceContribution) addRadiance(value Vec3, depth uint32, environment bool) {
	switch {
	case depth > 0:
		r.reflected = Add(r.reflected, value)
	case environment:
		r.environment = Add(r.environment, value)
	default:
		r.direct = Add(r.direct, value)
	}
}
