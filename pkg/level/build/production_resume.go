//go:build !js

package levelbuild

import (
	"fmt"
	"slices"

	"github.com/karty-game/ashura/internal/lightmapper"
	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

type productionResume struct {
	SceneHash    string              `json:"sceneHash"`
	ReceiverHash string              `json:"receiverHash"`
	Options      bake.Options        `json:"options"`
	Backend      string              `json:"backend"`
	Linear       assetbuild.Artifact `json:"linear"`
}

// planResumeInputs imports only explicitly named project sources. It neither
// searches previous build caches nor overrides ordinary processor identities.
func (plugin *productionPlugin) planResumeInputs(plan *assetbuild.Plan) error {
	plugin.resume = nil
	if len(plugin.options.ResumeSamples) > 0 && plugin.options.Adaptive != nil {
		return fmt.Errorf("sample resume requires fixed sampling; adaptive allocation metadata is unavailable: %w", ErrInvalid)
	}

	unique := make(map[string]bool)

	for _, pattern := range plugin.options.ResumeSamples {
		paths, err := plan.Glob(pattern)
		if err != nil {
			return err
		}

		for _, path := range paths {
			unique[path] = true
		}
	}

	paths := make([]string, 0, len(unique))
	for path := range unique {
		paths = append(paths, path)
	}

	slices.Sort(paths)

	for _, path := range paths {
		refs, err := plan.Add(assetbuild.Action{
			ID: "level/production/resume/" + path, Processor: "lightmapper-resume-input", Revision: "typed-linear@1",
			Options: struct {
				Path string `json:"path"`
			}{path},
			Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.lightmapper-resume-input@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := ctx.ReadFile(path)
				if err != nil {
					return nil, err
				}

				decoded, err := bake.DecodeResult(data)
				if err != nil {
					return nil, fmt.Errorf("resume input %q: %w", path, err)
				}

				artifact, err := ctx.StoreArtifact(data)
				if err != nil {
					return nil, err
				}

				return result(productionResume{
					SceneHash: decoded.SceneHash, ReceiverHash: decoded.ReceiverHash,
					Options: decoded.Options, Backend: decoded.Backend, Linear: artifact,
				})
			},
		})
		if err != nil {
			return err
		}

		plugin.resume = append(plugin.resume, refs[0])
	}

	return nil
}

func (plugin *productionPlugin) resumeSample(
	ctx *assetbuild.Context,
	scene *bake.PreparedScene,
	receivers []bake.Receiver,
) (bake.Result, bool, error) {
	if len(plugin.resume) == 0 {
		return bake.Result{}, false, nil
	}

	if scene == nil || plugin.worker == nil || plugin.options.Adaptive != nil {
		return bake.Result{}, false, ErrInvalid
	}

	receiverHash, err := lightmapper.ReceiverHash(receivers)
	if err != nil {
		return bake.Result{}, false, err
	}

	backend := plugin.worker.Device().Adapter
	if backend == "" {
		return bake.Result{}, false, ErrInvalid
	}

	records := make([]productionResume, len(plugin.resume))
	for index, ref := range plugin.resume {
		if err := readRecord(ctx, ref, &records[index]); err != nil {
			return bake.Result{}, false, err
		}
	}

	artifact, found, err := selectResumeArtifact(records, scene.Hash(), receiverHash, plugin.options.Sampling, backend)
	if err != nil || !found {
		return bake.Result{}, found, err
	}

	data, err := ctx.ReadArtifact(artifact)
	if err != nil {
		return bake.Result{}, false, err
	}

	decoded, err := bake.DecodeResult(data)
	if err != nil {
		return bake.Result{}, false, err
	}

	if err = validateResumeResult(decoded, scene.Hash(), receiverHash, plugin.options.Sampling, backend, receivers); err != nil {
		return bake.Result{}, false, err
	}

	return decoded, true, nil
}

func selectResumeArtifact(
	records []productionResume,
	sceneHash, receiverHash string,
	options bake.Options,
	backend string,
) (assetbuild.Artifact, bool, error) {
	var selected assetbuild.Artifact

	found := false

	for _, record := range records {
		if record.SceneHash != sceneHash || record.ReceiverHash != receiverHash || record.Options != options || record.Backend != backend {
			continue
		}

		if found && record.Linear.SHA256 != selected.SHA256 {
			return assetbuild.Artifact{}, false, fmt.Errorf("distinct compatible resume sample artifacts are ambiguous: %w", ErrInvalid)
		}

		selected, found = record.Linear, true
	}

	return selected, found, nil
}

func validateResumeResult(
	result bake.Result,
	sceneHash, receiverHash string,
	options bake.Options,
	backend string,
	receivers []bake.Receiver,
) error {
	if result.SceneHash != sceneHash || result.ReceiverHash != receiverHash || result.Options != options || result.Backend != backend ||
		options.Samples == 0 || len(result.Estimates) != len(receivers) {
		return fmt.Errorf("resume result provenance/options/count mismatch: %w", ErrInvalid)
	}

	for index, estimate := range result.Estimates {
		if estimate.ReceiverID != receivers[index].ID || estimate.Count != options.Samples ||
			(estimate.Count >= 2 && !estimate.VarianceValid) {
			return fmt.Errorf("resume result receiver order, fixed count or stable variance mismatch: %w", ErrInvalid)
		}
	}

	return nil
}
