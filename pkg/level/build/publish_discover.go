//go:build !js

package levelbuild

import (
	"fmt"
	"slices"
	"strings"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func discoverPublishedPages(ctx *assetbuild.Context, child *assetbuild.Plan, resource assetbuild.Resource, options PublishOptions) error {
	var record partitioned
	if err := readRecord(ctx, resource.Ref, &record); err != nil {
		return err
	}

	pageRefs, err := planPublishedPages(child, resource, record, options)
	if err != nil {
		return err
	}

	inputs := append([]assetbuild.Ref{resource.Ref}, pageRefs...)

	return child.Expand(
		assetbuild.Expansion{
			ID:     "level/publish-files/" + resource.ID,
			Inputs: inputs,
			Run: func(ctx *assetbuild.Context, filesPlan *assetbuild.Plan) error {
				return discoverConsolidatedChunks(ctx, filesPlan, resource, record, pageRefs, options)
			},
		},
	)
}

func planPublishedPages(
	child *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	options PublishOptions,
) ([]assetbuild.Ref, error) {
	var pageRefs []assetbuild.Ref

	for _, chunk := range record.Chunks {
		for index, page := range chunk.Pages {
			page, err := selectPublishedRepresentation(page, options.Representation)
			if err != nil {
				return nil, err
			}

			artifacts := []assetbuild.Artifact{page.Geometry}
			if page.UVLayout.SHA256 != "" {
				artifacts = append(artifacts, page.UVLayout)
			} else {
				artifacts = append(artifacts, page.BakeInputs)
			}

			artifacts = append(artifacts, page.Maps[:(leveldata.Page{Representation: page.Representation}).MapCount()]...)
			for _, texture := range page.SSBump {
				artifacts = append(artifacts, texture)
			}

			slices.SortFunc(artifacts, func(a, b assetbuild.Artifact) int {
				return strings.Compare(a.SHA256, b.SHA256)
			})

			refs, err := options.sharedPages.add(child,
				assetbuild.Action{
					Parallel:  options.NativeTextures != nil,
					ID:        fmt.Sprintf("level/package-page/%s/%s/%d", resource.ID, chunk.Description.ID, index),
					Processor: "level-package-page",
					Revision:  "shared-profile-pages@3",
					Artifacts: artifacts,
					Options:   pagePackagingOptions{chunk.Description.Origin, options.Development, page},
					Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.level-page@1"}},
					Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
						value, err := packagePage(ctx, page, chunk.Description.Origin, options.Development)
						if err != nil {
							return nil, err
						}

						return result(value)
					},
				},
			)
			if err != nil {
				return nil, err
			}

			pageRefs = append(pageRefs, refs[0])
		}
	}

	return pageRefs, nil
}
