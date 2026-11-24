//go:build !js

package levelbuild

import (
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/levelpack"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func discoverConsolidatedChunks(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	pages []assetbuild.Ref,
	options PublishOptions,
) error {
	chunks, err := planConsolidatedChunks(ctx, plan, resource, record, pages, options)
	if err != nil {
		return err
	}

	return plan.Expand(assetbuild.Expansion{
		ID:     "level/consolidated-files/" + resource.ID,
		Inputs: append([]assetbuild.Ref{resource.Ref}, chunks...),
		Run: func(ctx *assetbuild.Context, plan *assetbuild.Plan) error {
			return discoverNativeHDR(ctx, plan, resource, record, chunks, options)
		},
	})
}

func planConsolidatedChunks(ctx *assetbuild.Context, plan *assetbuild.Plan, resource assetbuild.Resource,
	record partitioned, pages []assetbuild.Ref, options PublishOptions) ([]assetbuild.Ref, error) {
	var chunks []assetbuild.Ref

	next := 0
	for _, chunk := range record.Chunks {
		refs := pages[next : next+len(chunk.Pages)]
		next += len(chunk.Pages)

		var (
			artifacts []assetbuild.Artifact
			inputs    []publishedPage
		)

		for _, ref := range refs {
			var input publishedPage
			if err := readRecord(ctx, ref, &input); err != nil {
				return nil, err
			}

			inputs = append(inputs, input)
			for _, file := range input.Files {
				artifacts = append(artifacts, file.Artifact)
			}
		}

		outputs, err := addConsolidation(plan, resource.ID, chunk.Description.ID, inputs, artifacts, options)
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, outputs[0])
	}

	return chunks, nil
}

func consolidatePages(ctx *assetbuild.Context, pages []publishedPage, maximum uint32) (assetbuild.Result, error) {
	inputs := make([]levelpack.Input, len(pages))
	shared := map[string]fileRecord{}

	for index, page := range pages {
		files := map[string]assetbuild.Artifact{}
		for _, file := range page.Files {
			files[file.Reference.Path] = file.Artifact
		}

		read := func(ref leveldata.Ref) ([]byte, error) {
			return ctx.ReadArtifact(files[ref.Path])
		}

		input, err := readConsolidationPage(page.Page, read)
		if err != nil {
			return nil, err
		}

		inputs[index] = input

		for _, material := range page.Page.Materials {
			for _, ref := range []*leveldata.Ref{material.Albedo, material.SSBump} {
				if ref != nil {
					shared[ref.Path] = fileRecord{Reference: *ref, Artifact: files[ref.Path]}
				}
			}
		}
	}

	packed, err := levelpack.Pack(ctx, inputs, int(maximum))
	if err != nil {
		return nil, err
	}

	output := make([]publishedPage, 0, len(packed))
	for _, atlas := range packed {
		page, err := storeConsolidatedPage(ctx, atlas, shared)
		if err != nil {
			return nil, err
		}

		output = append(output, page)
	}

	return result(output)
}

func readConsolidationPage(page leveldata.Page, read func(leveldata.Ref) ([]byte, error)) (levelpack.Input, error) {
	output := levelpack.Input{
		Preview:        page.Preview,
		Representation: page.Representation,
		Materials:      page.Materials,
		Scale:          page.Scale,
		Encoding:       page.Encoding,
	}

	data, err := read(page.Geometry)
	if err != nil {
		return output, err
	}

	output.Model, err = modeldata.Decode(data)
	if err != nil {
		return output, err
	}

	coordinates, err := read(page.UV)
	if err != nil {
		return output, err
	}

	layout, err := leveldata.DecodeUV(coordinates)
	if err != nil || layout.ModelDigest() != sha256.Sum256(data) || layout.PartCount() != len(output.Model.Parts) {
		return output, ErrInvalid
	}

	for index := range layout.PartCount() {
		output.UVs = append(output.UVs, layout.PartUV(index))
	}

	for index, ref := range page.Maps[:page.MapCount()] {
		data, err := read(ref)
		if err != nil {
			return output, err
		}

		output.Maps[index], err = ktx2data.Decode(data)
		if err != nil || output.Maps[index].Width != int(page.Width) || output.Maps[index].Height != int(page.Height) {
			return output, ErrInvalid
		}
	}

	return output, nil
}

//nolint:funlen,gosec // levelpack admits the output atlas dimension in [16, 2048].
func storeConsolidatedPage(ctx *assetbuild.Context, atlas levelpack.Output, shared map[string]fileRecord) (publishedPage, error) {
	output := publishedPage{
		Page: leveldata.Page{
			Preview:        atlas.Preview,
			Representation: atlas.Representation,
			Materials:      atlas.Materials,
			Scale:          atlas.Scale,
			Encoding:       atlas.Encoding,
			Width:          uint32(atlas.Size),
			Height:         uint32(atlas.Size),
		},
	}

	geometry, err := modeldata.EncodeWithBounds(atlas.Model)
	if err != nil {
		return output, err
	}

	layout, err := leveldata.EncodeUV(sha256.Sum256(geometry), atlas.UVs)
	if err != nil {
		return output, err
	}

	for index, payload := range [][]byte{geometry, layout} {
		extension := ".kmodel"
		if index == 1 {
			extension = ".kuv"
		}

		file, err := storePublished(ctx, "chunks", extension, payload)
		if err != nil {
			return output, err
		}

		if index == 0 {
			output.Page.Geometry = file.Reference
		} else {
			output.Page.UV = file.Reference
		}

		output.Files = append(output.Files, file)
	}

	for index, pixels := range atlas.Maps[:output.Page.MapCount()] {
		data, err := assetbuild.EncodeTexture(assetbuild.Image{Name: "RNM", Source: "rnm.png", Usage: assetbuild.UsageLightmap,
			Width: atlas.Size, Height: atlas.Size, Levels: [][]byte{pixels}})
		if err != nil {
			return output, err
		}

		file, err := storePublished(ctx, "lightmaps", ".ktx2", data)
		if err != nil {
			return output, err
		}

		output.Page.Maps[index] = file.Reference
		output.Files = append(output.Files, file)
	}

	for _, material := range atlas.Materials {
		for _, ref := range []*leveldata.Ref{material.Albedo, material.SSBump} {
			if ref != nil {
				file, found := shared[ref.Path]
				if !found {
					return output, fmt.Errorf("missing consolidated material: %w", ErrInvalid)
				}

				output.Files = append(output.Files, file)
			}
		}
	}

	return output, nil
}

type invalidAtlasPublisher struct{}

func (invalidAtlasPublisher) Name() string                { return levelPublishPluginName }
func (invalidAtlasPublisher) Plan(*assetbuild.Plan) error { return ErrInvalid }

// atlasActionCache shares an identical offline consolidation across delivery
// variants. Exact page metadata and payload artifact identities bind the action;
// the profile-specific metadata producer's name is not part of the conversion.
// This owns no payloads and does not combine chunks or encoding families.
type atlasActionCache struct{ refs map[string][]assetbuild.Ref }

func (shared *atlasActionCache) add(plan *assetbuild.Plan, action assetbuild.Action) ([]assetbuild.Ref, error) {
	if shared == nil {
		return plan.Add(action)
	}

	key, err := codecActionIdentity(action)
	if err != nil {
		return nil, err
	}

	if refs, found := shared.refs[key]; found {
		return slices.Clone(refs), nil
	}

	action.ID = "level/shared-atlas/" + key

	refs, err := plan.Add(action)
	if err != nil {
		return nil, err
	}

	if shared.refs == nil {
		shared.refs = make(map[string][]assetbuild.Ref)
	}

	shared.refs[key] = slices.Clone(refs)

	return refs, nil
}

func addConsolidation(
	plan *assetbuild.Plan,
	resourceID, chunkID string,
	inputs []publishedPage,
	artifacts []assetbuild.Artifact,
	options PublishOptions,
) ([]assetbuild.Ref, error) {
	return options.sharedAtlases.add(plan, assetbuild.Action{
		Parallel: true,
		ID:       "level/consolidate/" + resourceID + "/" + chunkID, Processor: "level-atlas", Revision: "charts-materials@3",
		Artifacts: artifacts,
		Options: struct {
			Maximum     uint32
			Development bool
			Pages       []publishedPage
		}{options.MaxAtlasSize, options.Development, inputs},
		Outputs: []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			if options.Development {
				return result(inputs)
			}

			return consolidatePages(ctx, inputs, options.MaxAtlasSize)
		},
	})
}
