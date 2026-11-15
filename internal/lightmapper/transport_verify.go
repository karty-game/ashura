package lightmapper

import "context"

// ReferenceResponses evaluates every requested local response normal using one
// set of footprint positions and transport paths. Verification callers supply
// an independent seed/pass; the normal list is not reconstructed from moments.
func ReferenceResponses(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	normals [][]Vec3,
) ([]ResponseEstimate, error) {
	if ctx == nil || scene == nil || len(receivers) != len(normals) {
		return nil, ErrInvalid
	}

	options, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}

	result := make([]ResponseEstimate, len(receivers))
	for index, receiver := range receivers {
		if err := validateResponseNormals(normals[index]); err != nil {
			return nil, err
		}

		result[index], err = bakeReferenceResponses(ctx, scene, receiver, options, normals[index])
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func validateResponseNormals(normals []Vec3) error {
	if len(normals) == 0 || len(normals) > 4096 {
		return ErrBounds
	}

	for _, normal := range normals {
		if !validVec(normal) || Length(normal) < .999 || Length(normal) > 1.001 {
			return ErrInvalid
		}
	}

	return nil
}

func bakeReferenceResponses(
	ctx context.Context,
	scene *PreparedScene,
	receiver Receiver,
	options Options,
	normals []Vec3,
) (ResponseEstimate, error) {
	estimate := ResponseEstimate{
		ReceiverID:         receiver.ID,
		Count:              options.Samples,
		Means:              make([]Vec3, len(normals)),
		SecondMoments:      make([]Vec3, len(normals)),
		Variance:           make([]Vec3, len(normals)),
		VarianceValid:      options.Samples > 1,
		SecondMomentsValid: true,
	}
	moments := make([][3]referenceWelford, len(normals))

	for sample := range options.Samples {
		if sample&63 == 0 {
			if err := ctx.Err(); err != nil {
				return ResponseEstimate{}, err
			}
		}

		values, err := referenceResponseSample(scene, receiver, options, normals, sample)
		if err != nil {
			return ResponseEstimate{}, err
		}

		for response, color := range values {
			for channel := range 3 {
				if err := moments[response][channel].add(color[channel], sample+1); err != nil {
					return ResponseEstimate{}, err
				}
			}
		}
	}

	for response := range estimate.Means {
		for channel := range 3 {
			mean, variance, second, valid, err := moments[response][channel].finish(options.Samples)
			if err != nil {
				return ResponseEstimate{}, err
			}

			estimate.Means[response][channel], estimate.Variance[response][channel], estimate.SecondMoments[response][channel] = mean, variance, second
			estimate.SecondMomentsValid = estimate.SecondMomentsValid && valid
		}
	}

	if !estimate.SecondMomentsValid {
		clear(estimate.SecondMoments)
	}

	return estimate, nil
}

//nolint:mnd // The independent reference proposal uses uniform-hemisphere density 1/(2pi).
func referenceResponseSample(scene *PreparedScene, receiver Receiver, options Options, normals []Vec3, sample uint32) ([]Vec3, error) {
	rng := func(bounce, dimension uint32) float32 {
		return SampleDimension(receiver.ID, options.Seed, sample, bounce, dimension)
	}

	surface, err := SampleReceiver(scene, receiver, rng(0, 0), rng(0, 1), rng(0, 2))
	if err != nil {
		return nil, err
	}

	values := make([]Vec3, len(normals))
	add := func(direction, incoming Vec3) {
		local := localDirection(surface, direction)
		for response, normal := range normals {
			values[response] = Add(values[response], Scale(incoming, max(0, Dot(normal, local))/lightPi))
		}
	}
	scene.primaryDirectSamples(surface, options, rng, func(direction, value Vec3, _ EmitterKind) { add(direction, value) })
	direction := sampleHemisphere(surface.GeometricNormal, rng(0, 6), rng(0, 7), false)
	pdf := 1 / (2 * lightPi)
	incoming := scene.traceReference(surface.Position, surface.GeometricNormal, direction, pdf, receiver.ID, sample, options)
	add(direction, Scale(Add(incoming.direct, Add(incoming.environment, incoming.reflected)), 1/pdf))

	return values, nil
}
