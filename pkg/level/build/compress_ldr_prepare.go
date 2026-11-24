//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func discoverNativeTextures(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	chunks []assetbuild.Ref,
	options PublishOptions,
) error {
	profile, err := normalizeNativeTextures(*options.NativeTextures)
	if err != nil {
		return err
	}

	normalized, err := planNormalizedLDR(ctx, plan, resource, chunks, options, profile)
	if err != nil {
		return err
	}

	return plan.Expand(
		assetbuild.Expansion{
			ID:     "level/normalized-ldr/" + resource.ID,
			Inputs: append([]assetbuild.Ref{resource.Ref}, normalized...),
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				return discoverEncodedNativeTextures(ctx, child, resource, record, normalized, options, profile)
			},
		},
	)
}

func planNormalizedLDR(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource,
	chunks []assetbuild.Ref, options PublishOptions, profile NativeTextureOptions) ([]assetbuild.Ref, error) {
	var normalized []assetbuild.Ref

	for index, source := range chunks {
		var pages []publishedPage
		if err := readRecord(ctx, source, &pages); err != nil {
			return nil, err
		}

		outputs, err := options.sharedCodecs.add(plan, assetbuild.Action{
			Parallel: true,
			ID: fmt.Sprintf(
				"level/prepare-ldr/%s/%d",
				resource.ID,
				index,
			),
			Processor: "level-lightmap-ldr",
			Revision:  "common-tuple-scale-transfer@1-wasi1",
			Inputs:    []assetbuild.Ref{source},
			Artifacts: publishedLightmapArtifacts(pages),
			Options:   profile.Transfer,
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run:       func(ctx *assetbuild.Context) (assetbuild.Result, error) { return prepareLDRPages(ctx, pages, profile) },
		})
		if err != nil {
			return nil, err
		}

		normalized = append(normalized, outputs[0])
	}

	return normalized, nil
}

func prepareLDRPages(ctx *assetbuild.Context, pages []publishedPage, profile NativeTextureOptions) (assetbuild.Result, error) {
	for index := range pages {
		if err := prepareLDRPage(ctx, &pages[index], profile); err != nil {
			return nil, err
		}
	}

	return result(pages)
}

func prepareLDRPage(ctx *assetbuild.Context, page *publishedPage, profile NativeTextureOptions) error {
	if page.Page.Encoding != rgbeEncoding ||
		(page.Page.Representation != leveldata.RGBDiffuse && page.Page.Representation != leveldata.RNMFit) {
		return fmt.Errorf("normalized LDR requires production coefficient inputs: %w", ErrInvalid)
	}

	sources := make([][]byte, page.Page.MapCount())
	for index := range sources {
		data, err := readPublishedLightmap(ctx, *page, index)
		if err != nil {
			return err
		}

		sources[index] = data
	}

	maps, scale, err := assetbuild.PrepareLDRLightmapCoefficients(sources, page.Page.Scale, nativeTextureSpace(profile))
	if err != nil {
		return err
	}

	if len(maps) != len(sources) {
		return ErrInvalid
	}

	replaced := make(map[string]bool, len(sources))
	for _, ref := range page.Page.Maps[:page.Page.MapCount()] {
		replaced[ref.SHA256] = true
	}

	files := withoutPublishedFiles(page.Files, replaced)

	for index, data := range maps {
		file, err := storePublished(ctx, "lightmaps", ".ktx2", data)
		if err != nil {
			return err
		}

		files = append(files, file)
		page.Page.Maps[index] = file.Reference
	}

	page.Files = files
	page.Page.Scale = scale
	page.Page.Encoding = profile.Transfer

	return nil
}
