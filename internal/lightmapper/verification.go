//go:build !js

package lightmapper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
)

const VerificationSchema = "karty.lightmapper-verification@1"

var ErrVerification = errors.New("lightmapper: directional representation verification failed")

// MaterialNormalDomain identifies actual prepared tangent normals. Domain
// identity must change with the source normal/height preparation recipe.
type MaterialNormalDomain struct {
	Material uint32
	Identity string
	Normals  []Vec3
}

// VerificationPolicy requests a separate fixed-count reference pass. A zero
// MaximumReceivers checks all receivers; a subset records its exact coverage.
type VerificationPolicy struct {
	Samples, Seed, MaximumReceivers                  uint32
	AbsoluteTolerance, RelativeTolerance, Confidence float32
	RequireAllReceivers, RequireMaterialDomain       bool
	MaterialDomains                                  []MaterialNormalDomain
}

type VerificationFailure struct {
	ReceiverID                              uint64
	Probe                                   uint32
	Normal                                  Vec3
	Reference, Reconstructed, StandardError Vec3
	AbsoluteError, RelativeError            float32
}

type VerificationResponse struct {
	ReceiverID                        uint64
	Count                             uint32
	Normals                           []Vec3
	Means, SecondMoments              []Vec3
	Variance                          []Vec3
	VarianceValid, SecondMomentsValid bool
}

type VerificationReport struct {
	Options                                                           Options
	Schema, SceneHash, ReceiverHash, PolicyHash, Backend, Calibration string
	Verified, AllReceivers, MaterialDomainVerified                    bool
	TotalReceivers, CheckedReceivers, Seed                            uint32
	TotalCoverage, CheckedCoverage                                    float64
	MaximumAbsoluteError, MaximumRelativeError                        float32
	SampledReceiverIDs                                                []uint64
	Failures                                                          []VerificationFailure
	Responses                                                         []VerificationResponse
}

type verificationPlan struct {
	Receivers []Receiver
	Estimates []Estimate
	Normals   [][]Vec3
	Report    VerificationReport
	Options   Options
}

//nolint:funlen,gosec,musttag // Complete probe qualification precedes dispatch. G115/G602: verificationInputsValid bounds receiver counts and admits equal estimate lengths before traversal. Policy hashing intentionally binds all exported policy fields, not a separate JSON wire schema.
func prepareVerification(
	scene *PreparedScene,
	receivers []Receiver,
	estimates []Estimate,
	options Options,
	policy VerificationPolicy,
	backend string,
) (verificationPlan, error) {
	var plan verificationPlan
	if !verificationInputsValid(scene, receivers, estimates, policy) {
		return plan, ErrInvalid
	}

	receiverHash, err := ReceiverHash(receivers)
	if err != nil {
		return plan, err
	}

	encoded, err := json.Marshal(policy)
	if err != nil {
		return plan, err
	}

	digest := sha256.Sum256(encoded)
	policyHash := hex.EncodeToString(digest[:])
	report := VerificationReport{
		Schema:                 VerificationSchema,
		SceneHash:              scene.Hash(),
		ReceiverHash:           receiverHash,
		PolicyHash:             policyHash,
		Backend:                backend,
		Calibration:            CalibrationProfile,
		TotalReceivers:         uint32(len(receivers)),
		MaterialDomainVerified: true,
	}
	report.Seed = adaptiveSeed(scene.Hash()+receiverHash+policyHash, "verification")

	domains, err := verificationDomains(scene, policy)
	if err != nil {
		return plan, err
	}

	count := uint32(len(receivers))
	if policy.MaximumReceivers > 0 {
		count = min(count, policy.MaximumReceivers)
	}

	if policy.RequireAllReceivers && count < uint32(len(receivers)) {
		return plan, ErrVerification
	}

	selected := verificationSelection(receivers, count, policyHash)

	probes := CalibrationProbes()

	for receiverIndex, receiver := range receivers {
		estimate := estimates[receiverIndex]
		if estimate.ReceiverID != receiver.ID || estimate.Count == 0 {
			return plan, ErrInvalid
		}

		for _, coefficient := range estimate.RNM {
			if !positiveVec(coefficient) {
				return plan, ErrInvalid
			}
		}

		if !receiverFinite(receiver.Coverage) || receiver.Coverage <= 0 {
			return plan, ErrInvalid
		}

		report.TotalCoverage += float64(receiver.Coverage)

		if !selected[receiverIndex] {
			continue
		}

		normals, verified, normalErr := verificationNormals(scene, receiver, domains, probes[:])
		if normalErr != nil {
			return plan, normalErr
		}

		report.MaterialDomainVerified = report.MaterialDomainVerified && verified

		plan.Receivers = append(plan.Receivers, receiver)
		plan.Estimates = append(plan.Estimates, estimate)
		plan.Normals = append(plan.Normals, normals)
		report.SampledReceiverIDs = append(report.SampledReceiverIDs, receiver.ID)
		report.CheckedCoverage += float64(receiver.Coverage)
	}

	report.CheckedReceivers = uint32(len(plan.Receivers))

	report.AllReceivers = report.CheckedReceivers == report.TotalReceivers
	if policy.RequireMaterialDomain && !report.MaterialDomainVerified {
		return plan, ErrVerification
	}

	plan.Report = report
	plan.Options = options
	plan.Options.Samples = policy.Samples
	plan.Options.Seed = report.Seed
	plan.Report.Options = plan.Options

	return plan, nil
}

func stringReceiverIdentity(id uint64) string {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], id)

	return hex.EncodeToString(data[:])
}
func verificationAppendNormal(normals []Vec3, normal Vec3) []Vec3 {
	if slices.Contains(normals, normal) {
		return normals
	}

	return append(normals, normal)
}

func finishVerification(plan verificationPlan, responses []ResponseEstimate, policy VerificationPolicy) (VerificationReport, error) {
	report := plan.Report
	if len(responses) != len(plan.Receivers) {
		return report, ErrInvalid
	}

	for receiverIndex, response := range responses {
		normals := plan.Normals[receiverIndex]
		if response.ReceiverID != plan.Receivers[receiverIndex].ID ||
			response.Count != policy.Samples ||
			len(response.Means) != len(normals) ||
			len(response.SecondMoments) != len(normals) ||
			len(response.Variance) != len(normals) ||
			!response.VarianceValid {
			return report, ErrInvalid
		}

		report.Responses = append(
			report.Responses,
			VerificationResponse{
				ReceiverID: response.ReceiverID,
				Count:      response.Count,
				Normals:    append([]Vec3(nil), normals...),
				Means:      append([]Vec3(nil), response.Means...),
				SecondMoments: append(
					[]Vec3(nil),
					response.SecondMoments...),
				Variance:           append([]Vec3(nil), response.Variance...),
				VarianceValid:      true,
				SecondMomentsValid: response.SecondMomentsValid,
			},
		)
		for probe, normal := range normals {
			if !positiveVec(response.Means[probe]) || !positiveVec(response.Variance[probe]) ||
				(response.SecondMomentsValid && !positiveVec(response.SecondMoments[probe])) {
				return report, ErrInvalid
			}

			failure, rejected := verificationCompare(plan.Estimates[receiverIndex], response, uint32(probe), normal, policy)

			report.MaximumAbsoluteError = max(report.MaximumAbsoluteError, failure.AbsoluteError)

			report.MaximumRelativeError = max(report.MaximumRelativeError, failure.RelativeError)
			if rejected {
				report.Failures = append(report.Failures, failure)
			}
		}
	}

	report.Verified = len(report.Failures) == 0 && (!policy.RequireAllReceivers || report.AllReceivers) &&
		(!policy.RequireMaterialDomain || report.MaterialDomainVerified)
	if !report.Verified {
		return report, ErrVerification
	}

	return report, nil
}

// VerifyReferenceRNM is the CPU correctness reference for directional
// verification. Production verification uses the hardware worker method.
func VerifyReferenceRNM(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	estimates []Estimate,
	options Options,
	policy VerificationPolicy,
) (VerificationReport, error) {
	plan, err := prepareVerification(scene, receivers, estimates, options, policy, "cpu-reference")
	if err != nil {
		return plan.Report, err
	}

	responses, err := ReferenceResponses(ctx, scene, plan.Receivers, plan.Options, plan.Normals)
	if err != nil {
		return plan.Report, err
	}

	return finishVerification(plan, responses, policy)
}

func verificationDomains(scene *PreparedScene, policy VerificationPolicy) (map[uint32]MaterialNormalDomain, error) {
	domains := make(map[uint32]MaterialNormalDomain, len(policy.MaterialDomains))
	for _, domain := range policy.MaterialDomains {
		if _, exists := domains[domain.Material]; exists || domain.Identity == "" || len(domain.Normals) == 0 ||
			int(domain.Material) >= len(scene.Source().Materials) {
			return nil, ErrInvalid
		}

		for _, normal := range domain.Normals {
			if !validVec(normal) || math.Abs(float64(Dot(normal, normal)-1)) > 1e-3 || normal[2] < 0 {
				return nil, ErrInvalid
			}
		}

		domains[domain.Material] = domain
	}

	return domains, nil
}

//nolint:gosec // G115: stratum endpoints are bounded by the admitted receiver slice length; integer products use uint64 before division.
func verificationSelection(receivers []Receiver, count uint32, policyHash string) []bool {
	selected := make([]bool, len(receivers))
	for stratum := range count {
		first := uint32(uint64(stratum) * uint64(len(receivers)) / uint64(count))
		last := uint32(uint64(stratum+1) * uint64(len(receivers)) / uint64(count))
		chosen := first
		best := uint32(math.MaxUint32)

		for receiverIndex := first; receiverIndex < last; receiverIndex++ {
			score := adaptiveSeed(policyHash+stringReceiverIdentity(receivers[receiverIndex].ID), "receiver-selection")
			if score < best {
				best, chosen = score, receiverIndex
			}
		}

		selected[chosen] = true
	}

	return selected
}

func verificationNormals(
	scene *PreparedScene,
	receiver Receiver,
	domains map[uint32]MaterialNormalDomain,
	probes []CalibrationProbe,
) ([]Vec3, bool, error) {
	verified := true

	normals := make([]Vec3, 0, len(probes))
	for _, probe := range probes {
		normals = append(normals, probe.Normal)
	}

	patches, patchErr := ReceiverPatches(scene, receiver)
	if patchErr != nil {
		return nil, false, patchErr
	}

	materials := make(map[uint32]bool)
	for _, patch := range patches {
		materials[patch.Material] = true
	}
	// Material IDs are emitted in order so map iteration cannot change sampling.
	for material := uint32(0); int(material) < len(scene.Source().Materials); material++ {
		if !materials[material] {
			continue
		}

		domain, exists := domains[material]
		if !exists {
			verified = false

			continue
		}

		for _, normal := range domain.Normals {
			normals = verificationAppendNormal(normals, normal)
		}
	}

	return normals, verified, nil
}

func verificationCompare(
	estimate Estimate,
	response ResponseEstimate,
	probe uint32,
	normal Vec3,
	policy VerificationPolicy,
) (VerificationFailure, bool) {
	weights := CalibrationWeights(normal)

	var reconstructed Vec3
	for basis := range 3 {
		reconstructed = Add(reconstructed, Scale(estimate.RNM[basis], weights[basis]))
	}

	failure := VerificationFailure{
		ReceiverID:    response.ReceiverID,
		Probe:         probe,
		Normal:        normal,
		Reference:     response.Means[probe],
		Reconstructed: reconstructed,
	}
	rejected := false

	for channel := range 3 {
		reference := float64(failure.Reference[channel])
		difference := math.Abs(float64(reconstructed[channel]) - reference)
		variance := float64(response.Variance[probe][channel])
		errorMean := math.Sqrt(variance / float64(response.Count))
		failure.StandardError[channel] = float32(errorMean)
		relative := difference / max(float64(policy.AbsoluteTolerance), reference)
		failure.AbsoluteError = max(failure.AbsoluteError, float32(difference))
		failure.RelativeError = max(failure.RelativeError, float32(relative))

		tolerance := max(float64(policy.AbsoluteTolerance), float64(policy.RelativeTolerance)*reference)
		if difference > tolerance+float64(policy.Confidence)*errorMean {
			rejected = true
		}
	}

	return failure, rejected
}

func verificationInputsValid(scene *PreparedScene, receivers []Receiver, estimates []Estimate, policy VerificationPolicy) bool {
	return scene != nil && len(receivers) > 0 && len(estimates) == len(receivers) && uint64(len(receivers)) <= math.MaxUint32 &&
		policy.Samples >= 2 && receiverFinite(policy.AbsoluteTolerance) && policy.AbsoluteTolerance > 0 &&
		receiverFinite(
			policy.RelativeTolerance,
		) && policy.RelativeTolerance >= 0 && receiverFinite(policy.Confidence) && policy.Confidence >= 0
}
