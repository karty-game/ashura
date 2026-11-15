package lightmapper

import (
	"context"
	"math"
)

type referenceContribution struct {
	moments                        Moments
	direct, environment, reflected Vec3
}

func misWeight(firstPDF, secondPDF float32) float32 {
	if firstPDF <= 0 {
		return 0
	}

	if secondPDF <= 0 {
		return 1
	}

	x, y := float64(firstPDF), float64(secondPDF)

	return float32(x * x / (x*x + y*y))
}
func (p *PreparedScene) rayOffset(position, normal Vec3) Vec3 {
	epsilon := float32(
		2e-5,
	) * max(
		1/p.source.MetresPerUnit,
		maxComponent(
			Vec3{float32(math.Abs(float64(position[0]))), float32(math.Abs(float64(position[1]))), float32(math.Abs(float64(position[2])))},
		),
	)

	return Add(position, Scale(normal, epsilon))
}

//nolint:mnd // The ray lower bound is expressed in meters then converted to scene units.
func (p *PreparedScene) rayMinimum() float32 { return 1e-6 / p.source.MetresPerUnit }
func localDirection(surface SurfaceSample, direction Vec3) Vec3 {
	tangent := Normalize(Sub(surface.Tangent, Scale(surface.Normal, Dot(surface.Tangent, surface.Normal))))
	if tangent == (Vec3{}) {
		tangent, _ = tangentFrame(surface.Normal)
	}

	hand := surface.Handedness
	if hand == 0 {
		hand = 1
	}

	bitangent := Scale(Cross(surface.Normal, tangent), hand)

	return Vec3{Dot(direction, tangent), Dot(direction, bitangent), Dot(direction, surface.Normal)}
}
func responseKernels(surface SurfaceSample, direction Vec3) [4]float32 {
	return CalibrationKernels(localDirection(surface, direction))
}
func addMoment(target *Moments, value Vec3, kernel [4]float32) {
	for i := range 4 {
		target[i] = Add(target[i], Scale(value, kernel[i]))
	}
}

func normalizeOptions(options Options) (Options, error) {
	if options.Outputs == 0 {
		options.Outputs = OutputRGB | OutputRNM
	}

	if options.Samples == 0 {
		options.Samples = 256
	}

	if options.Outputs&^(OutputRGB|OutputRNM) != 0 || options.Samples > 1<<24 || options.Bounces > 32 || options.RouletteDepth > 32 {
		return options, ErrBounds
	}

	return options, nil
}

// BakeReference is an explicit deterministic CPU correctness oracle, never a
// silent substitute for the production hardware compute worker.
func BakeReference(ctx context.Context, prepared *PreparedScene, receivers []Receiver, options Options) (Result, error) {
	if ctx == nil || prepared == nil {
		return Result{}, ErrInvalid
	}

	options, err := normalizeOptions(options)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Schema:    ResultSchema,
		Algorithm: Algorithm,
		SceneHash: prepared.Hash(),
		Options:   options,
		Backend:   "deterministic-cpu-reference",
		Estimates: make([]Estimate, len(receivers)),
	}

	result.ReceiverHash, err = ReceiverHash(receivers)
	if err != nil {
		return Result{}, err
	}

	for receiverIndex, receiver := range receivers {
		estimate, err := bakeReferenceReceiver(ctx, prepared, receiver, options)
		if err != nil {
			return Result{}, err
		}

		result.Estimates[receiverIndex] = estimate
	}

	return result, nil
}

//nolint:mnd // The receiver proposal uses the fixed uniform-hemisphere solid angle 2pi.
func referenceSample(prepared *PreparedScene, receiver Receiver, options Options, index uint32) (referenceContribution, error) {
	var result referenceContribution

	rng := func(bounce, dim uint32) float32 {
		return SampleDimension(receiver.ID, options.Seed, index, bounce, dim)
	}

	surface, err := SampleReceiver(prepared, receiver, rng(0, 0), rng(0, 1), rng(0, 2))
	if err != nil {
		return result, err
	}

	prepared.directPrimary(surface, options, rng, &result)
	direction := sampleHemisphere(surface.GeometricNormal, rng(0, 6), rng(0, 7), false)
	pdf := 1 / (2 * lightPi)
	incoming := prepared.traceReference(surface.Position, surface.GeometricNormal, direction, pdf, receiver.ID, index, options)

	kernel := responseKernels(surface, direction)
	for i := range kernel {
		kernel[i] /= pdf
	}

	addMoment(&result.moments, Add(incoming.direct, Add(incoming.environment, incoming.reflected)), kernel)
	result.direct = Add(result.direct, Scale(incoming.direct, kernel[0]))
	result.environment = Add(result.environment, Scale(incoming.environment, kernel[0]))
	result.reflected = Add(result.reflected, Scale(incoming.reflected, kernel[0]))

	return result, nil
}

func (p *PreparedScene) directPrimary(
	surface SurfaceSample,
	options Options,
	rng func(uint32, uint32) float32,
	result *referenceContribution,
) {
	p.primaryDirectSamples(surface, options, rng, func(direction, value Vec3, kind EmitterKind) {
		kernel := responseKernels(surface, direction)
		addMoment(&result.moments, value, kernel)

		if kind == EnvironmentSource {
			result.environment = Add(result.environment, Scale(value, kernel[0]))
		} else {
			result.direct = Add(result.direct, Scale(value, kernel[0]))
		}
	})
}

//nolint:mnd // Uniform hemisphere density is 1/(2pi); the shadow endpoint allowance excludes the sampled emitter.
func (p *PreparedScene) primaryDirectSamples(
	surface SurfaceSample,
	options Options,
	rng func(uint32, uint32) float32,
	consume func(Vec3, Vec3, EmitterKind),
) {
	visit := func(index uint32, mass float32) {
		light := p.SampleEmitter(surface.Position, index, rng(0, 4), rng(0, 5))
		if light.PDF <= 0 || mass <= 0 || Dot(surface.GeometricNormal, light.Direction) <= 0 || maxComponent(light.Radiance) <= 0 {
			return
		}

		if p.Occluded(
			Ray{
				Origin:    p.rayOffset(surface.Position, surface.GeometricNormal),
				Direction: light.Direction,
				Min:       p.rayMinimum(),
				Max:       light.Distance * (1 - 2e-5),
			},
		) {
			return
		}

		weight := float32(1)
		if !light.Delta && p.source.Emitters[index].Kind != Sun {
			weight = misWeight(mass*light.PDF, 1/(2*lightPi))
		}

		value := Scale(light.Radiance, weight/(mass*light.PDF))
		consume(light.Direction, value, p.source.Emitters[index].Kind)
	}

	if options.ExactLights {
		for i := range p.source.Emitters {
			visit(uint32(i), 1)
		}
	} else {
		index, mass := p.SampleLight(surface.Position, rng(0, 3))
		if mass > 0 {
			visit(index, mass)
		}
	}
}
