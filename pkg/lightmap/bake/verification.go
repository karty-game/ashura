//go:build !js

package lightmapbake

import (
	"context"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
)

type MaterialNormalDomain = lightmapper.MaterialNormalDomain
type VerificationPolicy = lightmapper.VerificationPolicy
type VerificationBatch = lightmapper.VerificationBatch
type VerificationReport = lightmapper.VerificationReport
type VerificationFailure = lightmapper.VerificationFailure
type VerificationResponse = lightmapper.VerificationResponse

var ErrVerification = lightmapper.ErrVerification

// VerifyReferenceRNM checks directional projection against a separately traced
// CPU reference. Production uses Worker's hardware VerifyRNM method.
func VerifyReferenceRNM(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	estimates []Estimate,
	options Options,
	policy VerificationPolicy,
) (VerificationReport, error) {
	return lightmapper.VerifyReferenceRNM(ctx, scene, receivers, estimates, options, policy)
}
