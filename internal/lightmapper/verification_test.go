//go:build !js

package lightmapper

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestVerificationChecksIndependentResponsesAndReportsMissingDomain(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()

	scene, err := PrepareScene(
		Scene{
			ID:            "verify",
			Space:         "world",
			MetresPerUnit: 1,
			Triangles:     []Triangle{triangle},
			Materials:     []Material{{Reflectance: Vec3{.5, .5, .5}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	receivers, err := PrepareReceivers([]Triangle{triangle}, [][3]Vec2{triangle.UV}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	estimates := []Estimate{{ReceiverID: receivers[0].ID, Count: 32, RNM: [3]Vec3{{1, 2, 3}, {2, 3, 4}, {3, 4, 5}}}}
	policy := VerificationPolicy{
		Samples:               128,
		AbsoluteTolerance:     .001,
		RelativeTolerance:     .001,
		RequireAllReceivers:   true,
		RequireMaterialDomain: true,
		MaterialDomains:       []MaterialNormalDomain{{Material: 0, Identity: "actual-normal-domain", Normals: []Vec3{{0, 0, 1}}}},
	}

	plan, err := prepareVerification(
		scene,
		receivers,
		estimates,
		Options{Outputs: OutputRNM, Samples: 32, Bounces: 3},
		policy,
		"hardware-test",
	)
	if err != nil {
		t.Fatal(err)
	}

	responses := []ResponseEstimate{
		{
			ReceiverID:         receivers[0].ID,
			Count:              128,
			Means:              make([]Vec3, len(plan.Normals[0])),
			SecondMoments:      make([]Vec3, len(plan.Normals[0])),
			Variance:           make([]Vec3, len(plan.Normals[0])),
			VarianceValid:      true,
			SecondMomentsValid: true,
		},
	}
	for probeIndex, normal := range plan.Normals[0] {
		weights := CalibrationWeights(normal)
		for basis := range 3 {
			responses[0].Means[probeIndex] = Add(responses[0].Means[probeIndex], Scale(estimates[0].RNM[basis], weights[basis]))
		}

		responses[0].SecondMoments[probeIndex] = Mul(responses[0].Means[probeIndex], responses[0].Means[probeIndex])
	}

	report, err := finishVerification(plan, responses, policy)
	if err != nil || !report.Verified || !report.AllReceivers || !report.MaterialDomainVerified || len(report.Responses[0].Normals) != 17 {
		t.Fatal(report, err)
	}
	// Four fitting moments are deliberately unchanged. An independently observed
	// directional response still exposes a bad fitted representation.
	responses[0].Means[7][0] += 1
	responses[0].SecondMoments[7] = Mul(responses[0].Means[7], responses[0].Means[7])

	report, err = finishVerification(plan, responses, policy)
	if err == nil || report.Verified || len(report.Failures) != 1 || report.Failures[0].Probe != 7 {
		t.Fatal(report, err)
	}

	policy.MaterialDomains = nil
	if _, err = prepareVerification(scene, receivers, estimates, Options{Outputs: OutputRNM}, policy, "hardware-test"); err == nil {
		t.Fatal("accepted final verification without material domain")
	}
}

func TestGPUResponseDecoderUsesAllProbesAndRejectsTruncation(t *testing.T) {
	t.Parallel()

	receivers := []Receiver{{ID: 7}, {ID: 8}}
	normals := [][]Vec3{{{0, 0, 1}}, {{0, 0, 1}, {.6, 0, .8}}}
	data := make([]byte, 2*(9*2+2)*4)

	for i := range receivers {
		offset := i * 20 * 4
		for probeIndex := range 12 {
			binary.LittleEndian.PutUint32(data[offset+probeIndex*4:], math.Float32bits(float32(probeIndex+1)))
		}

		binary.LittleEndian.PutUint32(data[offset+18*4:], 128)
		binary.LittleEndian.PutUint32(data[offset+19*4:], 3)
	}

	responses, err := DecodeGPUResponses(data, receivers, normals, 2)
	if err != nil || len(responses[0].Means) != 1 || len(responses[1].Means) != 2 || responses[1].Means[1][2] != 6 ||
		responses[0].SecondMoments[0][0] != 7 {
		t.Fatal(responses, err)
	}

	if _, err = DecodeGPUResponses(data[:len(data)-1], receivers, normals, 2); err == nil {
		t.Fatal("accepted truncated response packet")
	}
}
