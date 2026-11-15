//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func discoverNativeHDR(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	chunks []assetbuild.Ref,
	options PublishOptions,
) error {
	if options.NativeTextures != nil {
		return discoverNativeTextures(ctx, plan, resource, record, chunks, options)
	}

	if options.NativeHDR == nil {
		return discoverAlbedoCompression(ctx, plan, resource, record, chunks, options)
	}

	if err := validateNativeHDR(*options.NativeHDR); err != nil {
		return err
	}

	var normalized []assetbuild.Ref

	for index, source := range chunks {
		var pages []publishedPage
		if err := readRecord(ctx, source, &pages); err != nil {
			return err
		}

		ref, err := addNormalizeHDRAction(plan, resource.ID, index, source, pages)
		if err != nil {
			return err
		}

		normalized = append(normalized, ref)
	}

	return plan.Expand(assetbuild.Expansion{
		ID:     "level/normalized-hdr/" + resource.ID,
		Inputs: append([]assetbuild.Ref{resource.Ref}, normalized...),
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			return discoverEncodedNativeHDR(ctx, child, resource, record, normalized, options)
		},
	})
}

func addNormalizeHDRAction(
	plan *assetbuild.Plan,
	identity string,
	index int,
	source assetbuild.Ref,
	pages []publishedPage,
) (assetbuild.Ref, error) {
	refs, err := plan.Add(assetbuild.Action{
		ID:        fmt.Sprintf("level/normalize-hdr/%s/%d", identity, index),
		Processor: "level-normalize-hdr",
		Revision:  "common-power-of-two@1-wasi1",
		Inputs:    []assetbuild.Ref{source},
		Artifacts: publishedLightmapArtifacts(pages),
		Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return normalizeHDRPages(ctx, pages)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func publishedLightmapArtifacts(pages []publishedPage) []assetbuild.Artifact {
	var artifacts []assetbuild.Artifact

	for _, page := range pages {
		for _, file := range page.Files {
			for _, ref := range page.Page.Maps[:page.Page.MapCount()] {
				if file.Reference == ref {
					artifacts = append(artifacts, file.Artifact)
				}
			}
		}
	}

	return artifacts
}

func normalizeHDRPages(ctx *assetbuild.Context, pages []publishedPage) (assetbuild.Result, error) {
	for index := range pages {
		if err := normalizeHDRPage(ctx, &pages[index]); err != nil {
			return nil, err
		}
	}

	return result(pages)
}

func normalizeHDRPage(ctx *assetbuild.Context, page *publishedPage) error {
	if page.Page.Encoding != rgbeEncoding {
		return ErrInvalid
	}

	maps := make([][]byte, page.Page.MapCount())

	for index := range maps {
		data, err := readPublishedLightmap(ctx, *page, index)
		if err != nil {
			return err
		}

		maps[index] = data
	}

	maps, scale, err := assetbuild.NormalizeRGBECoefficients(maps, page.Page.Scale)
	if err != nil {
		return err
	}

	old := map[string]bool{}
	for _, ref := range page.Page.Maps[:page.Page.MapCount()] {
		old[ref.SHA256] = true
	}

	keep := withoutPublishedFiles(page.Files, old)

	for index, data := range maps {
		file, err := storePublished(ctx, "lightmaps", ".ktx2", data)
		if err != nil {
			return err
		}

		keep = append(keep, file)
		page.Page.Maps[index] = file.Reference
	}

	page.Files = keep
	page.Page.Scale = scale

	return nil
}

func readPublishedLightmap(ctx *assetbuild.Context, page publishedPage, index int) ([]byte, error) {
	for _, file := range page.Files {
		if file.Reference == page.Page.Maps[index] {
			return ctx.ReadArtifact(file.Artifact)
		}
	}

	return nil, ErrInvalid
}
