package lightmapper

import "context"

func bakeReferenceReceiver(ctx context.Context, scene *PreparedScene, receiver Receiver, options Options) (Estimate, error) {
	estimate := Estimate{ReceiverID: receiver.ID, Count: options.Samples, VarianceValid: options.Samples > 1, SecondMomentsValid: true}

	var (
		moments     [4][3]referenceWelford
		diagnostics [3][3]float64
	)

	for sample := range options.Samples {
		if sample&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Estimate{}, err
			}
		}

		value, err := referenceSample(scene, receiver, options, sample)
		if err != nil {
			return Estimate{}, err
		}

		if err = addReferenceSample(&moments, &diagnostics, value, sample+1); err != nil {
			return Estimate{}, err
		}
	}

	if err := resolveReferenceMoments(&estimate, &moments, &diagnostics, options.Samples); err != nil {
		return Estimate{}, err
	}

	if options.Outputs&OutputRNM != 0 {
		var err error

		estimate.RNM, err = ProjectRNM(Moments(estimate.Statistics))
		if err != nil {
			return Estimate{}, err
		}
	}

	return estimate, nil
}

func addReferenceSample(moments *[4][3]referenceWelford, diagnostics *[3][3]float64, value referenceContribution, count uint32) error {
	for moment := range 4 {
		for channel := range 3 {
			if err := moments[moment][channel].add(value.moments[moment][channel], count); err != nil {
				return err
			}
		}
	}

	for kind, color := range [3]Vec3{value.direct, value.environment, value.reflected} {
		for channel := range 3 {
			diagnostics[kind][channel] += float64(color[channel])
		}
	}

	return nil
}

func resolveReferenceMoments(estimate *Estimate, moments *[4][3]referenceWelford, diagnostics *[3][3]float64, count uint32) error {
	for moment := range 4 {
		for channel := range 3 {
			mean, variance, second, valid, err := moments[moment][channel].finish(count)
			if err != nil {
				return err
			}

			estimate.Statistics[moment][channel], estimate.Variance[moment][channel], estimate.SecondMoments[moment][channel] = mean, variance, second
			estimate.SecondMomentsValid = estimate.SecondMomentsValid && valid
		}
	}

	if !estimate.SecondMomentsValid {
		estimate.SecondMoments = [4]Vec3{}
	}

	for kind, target := range []*Vec3{&estimate.Direct, &estimate.Environment, &estimate.Reflected} {
		for channel := range 3 {
			target[channel] = float32(diagnostics[kind][channel] / float64(count))
		}
	}

	return nil
}
