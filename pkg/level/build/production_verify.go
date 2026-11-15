//go:build !js

package levelbuild

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func (plugin *productionPlugin) verifyProductionPages(
	ctx *assetbuild.Context,
	scene *bake.PreparedScene,
	sets []bake.ReceiverSet,
	linear []bake.Result,
) ([]assetbuild.Artifact, error) {
	if len(sets) != len(linear) {
		return nil, ErrInvalid
	}

	quality := make([]assetbuild.Artifact, len(sets))

	if plugin.options.Verification == nil {
		return quality, nil
	}

	policy := *plugin.options.Verification

	if !plugin.options.Preview {
		policy.RequireMaterialDomain, policy.RequireAllReceivers = true, true
	}

	batches, pages := productionVerificationBatches(sets, linear, policy)

	if len(batches) == 0 {
		return quality, nil
	}

	if err := plugin.openWorker(ctx); err != nil {
		return nil, err
	}

	reports, err := plugin.worker.VerifyRNMBatches(ctx, scene, batches)

	if err != nil && (!plugin.options.Preview || !errors.Is(err, bake.ErrVerification)) {
		var report bake.VerificationReport

		for _, candidate := range reports {
			if !candidate.Verified {
				report = candidate

				break
			}
		}

		return nil, productionVerificationError(report, err)
	}

	if len(reports) != len(pages) {
		return nil, ErrInvalid
	}

	for index, report := range reports {
		if !plugin.options.Preview && !report.Verified {
			return nil, fmt.Errorf("directional output did not meet declared quality: %w", ErrInvalid)
		}

		quality[pages[index]], err = storeProductionVerification(ctx, report)
		if err != nil {
			return nil, err
		}
	}

	return quality, nil
}

func productionVerificationError(report bake.VerificationReport, err error) error {
	return fmt.Errorf(
		"directional verification: %d failures, maximum absolute %g relative %g, material domain %t, all receivers %t: %w",
		len(report.Failures),
		report.MaximumAbsoluteError,
		report.MaximumRelativeError,
		report.MaterialDomainVerified,
		report.AllReceivers,
		err,
	)
}

// The versioned typed verification record has canonical exported field names.
func storeProductionVerification(ctx *assetbuild.Context, report bake.VerificationReport) (assetbuild.Artifact, error) {
	encoded, err := json.Marshal(report)
	if err != nil {
		return assetbuild.Artifact{}, err
	}

	return ctx.StoreArtifact(encoded)
}

func productionVerificationBatches(
	sets []bake.ReceiverSet,
	linear []bake.Result,
	policy bake.VerificationPolicy,
) ([]bake.VerificationBatch, []int) {
	batches := make([]bake.VerificationBatch, 0, len(sets))
	pages := make([]int, 0, len(sets))

	for index, set := range sets {
		if linear[index].Options.Outputs&bake.OutputRNM == 0 {
			continue
		}

		batches = append(
			batches,
			bake.VerificationBatch{
				Receivers: set.Receivers,
				Estimates: linear[index].Estimates,
				Options:   linear[index].Options,
				Policy:    policy,
			},
		)
		pages = append(pages, index)
	}

	return batches, pages
}
