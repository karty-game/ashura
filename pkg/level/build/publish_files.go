//go:build !js

package levelbuild

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func sortedFiles(files map[string]fileRecord) []fileRecord {
	result := make([]fileRecord, 0, len(files))
	for _, file := range files {
		result = append(result, file)
	}

	slices.SortFunc(result, func(a, b fileRecord) int { return cmp.Compare(a.Reference.Path, b.Reference.Path) })

	return result
}
func addFile(plan *assetbuild.Plan, level string, file fileRecord) error {
	refs, err := plan.Add(
		assetbuild.Action{
			Parallel:  true,
			ID:        "level/file/" + level + "/" + file.Reference.Path,
			Processor: "level-external-file",
			Revision:  "immutable-payload@1",
			Artifacts: []assetbuild.Artifact{file.Artifact},
			Options:   file.Reference,
			Outputs: []assetbuild.Slot{
				{Name: filesSlot, Schema: filesSchema},
			},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := ctx.ReadArtifact(file.Artifact)
				if err != nil {
					return nil, err
				}

				if len(data) != file.Reference.Size {
					return nil, fmt.Errorf("external payload size: %w", ErrInvalid)
				}

				encoded, err := assetbuild.EncodeBundle(assetbuild.Bundle{"public/" + file.Reference.Path: data})

				return assetbuild.Result{filesSlot: encoded}, err
			},
		},
	)
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}
