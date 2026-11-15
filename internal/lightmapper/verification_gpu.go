//go:build !js

package lightmapper

import "context"

// VerifyRNM independently estimates complete directional responses on hardware;
// fitting statistics are never treated as a reconstruction-error measurement.
func (worker *GPUWorker) VerifyRNM(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	estimates []Estimate,
	options Options,
	policy VerificationPolicy,
) (VerificationReport, error) {
	if worker == nil {
		return VerificationReport{}, ErrInvalid
	}

	plan, err := prepareVerification(scene, receivers, estimates, options, policy, worker.Device().Adapter)
	if err != nil {
		return plan.Report, err
	}

	responses, err := worker.BakeResponses(ctx, scene, plan.Receivers, plan.Options, plan.Normals)
	if err != nil {
		return plan.Report, err
	}

	return finishVerification(plan, responses, policy)
}
