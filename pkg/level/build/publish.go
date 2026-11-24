//go:build !js

package levelbuild

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"go/format"
	"go/token"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

const levelPublishPluginName = "level-publish"

type fileRecord struct {
	Reference leveldata.Ref
	Artifact  assetbuild.Artifact
}
type publishedPage struct {
	Page  leveldata.Page
	Files []fileRecord
}

// Publish emits content-addressed external chunk resources and a small typed descriptor.
func Publish(options PublishOptions) assetbuild.Plugin {
	options, invalid := preparePublishOptions(options)
	if invalid != nil {
		return invalid
	}

	stageName := "publish"
	if options.Variant != "" {
		stageName += "-" + options.Variant
	}

	return stage{name: stageName, schema: bakedSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		if options.Variant != "" {
			resource.ID += "/" + options.Variant
		}

		return plan.Expand(
			assetbuild.Expansion{
				ID:     "level/publish/" + resource.ID,
				Inputs: []assetbuild.Ref{resource.Ref},
				Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
					return discoverPublishedPages(ctx, child, resource, options)
				},
			},
		)
	}}
}

func preparePublishOptions(options PublishOptions) (PublishOptions, assetbuild.Plugin) {
	if options.NativeHDR != nil && options.NativeTextures != nil {
		return options, invalidNativePublisher{err: fmt.Errorf("NativeHDR and NativeTextures are mutually exclusive: %w", ErrInvalid)}
	}

	if options.NativeTextures != nil {
		profile, err := normalizeNativeTextures(*options.NativeTextures)
		if err != nil {
			return options, invalidNativePublisher{err: err}
		}

		if profile.NativeAlbedo && options.AlbedoETC1S == nil {
			return options, invalidNativePublisher{err: fmt.Errorf("native albedos require offline ETC1S preparation: %w", ErrInvalid)}
		}

		options.NativeTextures = &profile
	}

	if options.Package == "" {
		options.Package = "levels"
	}

	if !validLevelIdentifier(options.Package) {
		return options, invalidLevelPublisher{packageName: options.Package}
	}

	if options.Variant != "" && !validLevelIdentifier(options.Variant) {
		return options, invalidLevelPublisher{variant: options.Variant}
	}

	if options.MaxAtlasSize == 0 {
		options.MaxAtlasSize = 1024
	}

	if options.MaxAtlasSize < 1024 || options.MaxAtlasSize > 2048 || options.MaxAtlasSize&(options.MaxAtlasSize-1) != 0 {
		return options, invalidAtlasPublisher{}
	}

	return options, nil
}

//nolint:funlen // Preserve the atomic geometry/UV/material publication boundary.
func packagePage(ctx *assetbuild.Context, page pageRecord, origin [3]float32, development bool) (publishedPage, error) {
	var output publishedPage

	admitted, err := readPageInputs(ctx, page)
	if err != nil {
		return output, err
	}

	doc, layout, input := admitted.model, admitted.layout, admitted.bytes

	output.Page = leveldata.Page{
		Representation: page.Representation,
		Preview:        page.Preview,
		Scale:          page.Scale,
		Encoding:       page.Encoding,
		Width:          page.Width,
		Height:         page.Height,
	}

	if err := packageMaterials(ctx, page, &doc, origin, &output); err != nil {
		return output, err
	}

	doc.Textures = nil

	geometry, err := modeldata.EncodeWithBounds(doc)
	if err != nil {
		return output, err
	}

	modelFile, err := storePublished(ctx, "chunks", ".kmodel", geometry)
	if err != nil {
		return output, err
	}

	output.Page.Geometry = modelFile.Reference
	output.Files = append(output.Files, modelFile)

	uvLayout, err := leveldata.EncodeUV(sha256.Sum256(geometry), layout.UVs)
	if err != nil {
		return output, err
	}

	uvFile, err := storePublished(ctx, "chunks", ".kuv", uvLayout)
	if err != nil {
		return output, err
	}

	output.Page.UV = uvFile.Reference
	output.Files = append(output.Files, uvFile)

	for index, artifact := range page.Maps[:output.Page.MapCount()] {
		data, err := ctx.ReadArtifact(artifact)
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

	if development && page.BakeInputs.SHA256 != "" {
		file, err := storePublished(ctx, "development", ".klightmap", input)
		if err != nil {
			return output, err
		}

		output.Files = append(output.Files, file)
		output.Page.BakeInputs = &file.Reference
	}

	return output, nil
}

func storePublished(ctx *assetbuild.Context, directory, extension string, data []byte) (fileRecord, error) {
	artifact, err := ctx.StoreArtifact(data)
	if err != nil {
		return fileRecord{}, err
	}

	ref, err := leveldata.Reference("levels/"+directory+"/"+artifact.SHA256+extension, data)

	return fileRecord{Reference: ref, Artifact: artifact}, err
}

func publishLevel(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	pageRefs []assetbuild.Ref,
	options PublishOptions,
) error {
	files := map[string]fileRecord{}

	document := leveldata.Document{
		Schema:        publicationSchema(options),
		Source:        record.Source,
		ChunkSize:     record.Size,
		TexelsPerUnit: record.Density,
		Objects:       record.Objects,
	}

	for index, chunk := range record.Chunks {
		description := chunk.Description

		var packed []publishedPage
		if err := readRecord(ctx, pageRefs[index], &packed); err != nil {
			return err
		}

		for _, page := range packed {
			placeProfileMaterials(&page, options.NativeTextures)
			document.Schema = pagePublicationSchema(document.Schema, page.Page)

			description.Pages = append(description.Pages, page.Page)
			for _, file := range page.Files {
				if old, ok := files[file.Reference.Path]; ok && old != file {
					return ErrInvalid
				}

				files[file.Reference.Path] = file
			}
		}

		document.Chunks = append(document.Chunks, description)
	}
	// Dynamic models retain their independent resource family and are never merged into static chunks.
	for _, placement := range record.Dynamic {
		ref := leveldata.Ref{
			Path:   "levels/models/" + placement.Asset.SHA256 + ".kmodel",
			Size:   placement.Asset.Size,
			SHA256: placement.Asset.SHA256,
		}

		files[ref.Path] = fileRecord{Reference: ref, Artifact: placement.Asset}
		for i := range document.Objects {
			if document.Objects[i].ID == placement.ID {
				descriptor := ref
				document.Objects[i].Asset = &descriptor
			}
		}
	}

	if options.BundleChunks {
		return planBundledLevel(plan, resource, record, document, files, options)
	}
	// Each payload is one independently bounded publication action, not a level-sized bundle.
	if err := publishSharedFiles(plan, resource.ID, files, options.sharedFiles); err != nil {
		return err
	}

	return publishLevelManifest(plan, resource, record, document, options)
}

func packageMaterials(ctx *assetbuild.Context, page pageRecord, doc *modeldata.Document, origin [3]float32, output *publishedPage) error {
	for i := range doc.Parts {
		part := &doc.Parts[i]
		source := part.Material

		material := leveldata.Material{Name: part.Name, Color: source.Color, DoubleSided: source.DoubleSided}
		if source.Albedo >= 0 {
			ref, err := storeSourceTexture(ctx, doc.Textures[source.Albedo].Data, texturedata.SRGB)
			if err != nil {
				return err
			}

			output.Files = append(output.Files, ref)
			material.Albedo = &ref.Reference
		}

		if texture, ok := page.SSBump[part.Name]; ok {
			data, err := ctx.ReadArtifact(texture)
			if err != nil {
				return err
			}

			ref, err := storeSourceTexture(ctx, data, texturedata.Linear)
			if err != nil {
				return err
			}

			output.Files = append(output.Files, ref)
			material.SSBump = &ref.Reference
		}

		if source.Normal >= 0 && material.SSBump == nil && page.Representation != leveldata.RGBDiffuse {
			return fmt.Errorf("%s has a normal map but no prepared Source SSBump override: %w", part.Name, ErrInvalid)
		}

		if source.AO >= 0 {
			return fmt.Errorf("%s mapped AO requires a prepared Source material profile: %w", part.Name, ErrInvalid)
		}

		output.Page.Materials = append(output.Page.Materials, material)

		for offset := 0; offset < len(part.Vertices); offset += 32 {
			for axis := range 3 {
				value := math.Float32frombits(binary.LittleEndian.Uint32(part.Vertices[offset+axis*4:]))
				binary.LittleEndian.PutUint32(part.Vertices[offset+axis*4:], math.Float32bits(value-origin[axis]))
			}
		}

		part.Material.Albedo = -1
		part.Material.Normal = -1
		part.Material.AO = -1
	}

	return nil
}

func storeSourceTexture(ctx *assetbuild.Context, data []byte, space texturedata.ColorSpace) (fileRecord, error) {
	prepared, err := ktx2data.Decode(data)
	if err != nil {
		return fileRecord{}, err
	}

	const maximumSourceDimension = 1024
	if prepared.ColorSpace != space || prepared.Width > maximumSourceDimension || prepared.Height > maximumSourceDimension {
		return fileRecord{}, fmt.Errorf(
			"source texture must use its declared color space and dimensions at most %d: %w",
			maximumSourceDimension,
			ErrInvalid,
		)
	}

	return storePublished(ctx, "textures", ".ktx2", data)
}

type admittedPageInputs struct {
	model  modeldata.Document
	layout *lightmapdata.Document
	bytes  []byte
}

func readPageInputs(ctx *assetbuild.Context, page pageRecord) (admittedPageInputs, error) {
	data, err := ctx.ReadArtifact(page.Geometry)
	if err != nil {
		return admittedPageInputs{}, err
	}

	doc, err := modeldata.Decode(data)
	if err != nil {
		return admittedPageInputs{}, err
	}

	if page.UVLayout.SHA256 != "" {
		raw, err := ctx.ReadArtifact(page.UVLayout)
		if err != nil {
			return admittedPageInputs{}, err
		}

		uvLayout, err := leveldata.DecodeUV(raw)
		if err != nil || uvLayout.ModelDigest() != sha256.Sum256(data) || uvLayout.PartCount() != len(doc.Parts) {
			return admittedPageInputs{}, ErrInvalid
		}

		layout := &lightmapdata.Document{Width: page.Width, Height: page.Height, ModelSHA256: sha256.Sum256(data)}
		for i := range uvLayout.PartCount() {
			layout.UVs = append(layout.UVs, uvLayout.PartUV(i))
		}

		return admittedPageInputs{model: doc, layout: layout}, nil
	}

	input, err := ctx.ReadArtifact(page.BakeInputs)
	if err != nil {
		return admittedPageInputs{}, err
	}

	layout, err := lightmapdata.Decode(input)
	if err != nil {
		return admittedPageInputs{}, err
	}

	return admittedPageInputs{model: doc, layout: layout, bytes: input}, nil
}

func publishLevelManifest(
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	document leveldata.Document,
	options PublishOptions,
) error {
	stem := strings.TrimSuffix(path.Base(record.Source), ".scene.yaml")
	if options.Variant != "" {
		stem += "_" + options.Variant
	}

	name := assetbuild.AssetName(stem)
	if !validLevelIdentifier(name) {
		return fmt.Errorf("invalid generated level descriptor name %q: %w", name, ErrInvalid)
	}

	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "level/manifest/" + resource.ID,
			Processor: "level-manifest",
			Revision:  "baked-level@1",
			Options: struct {
				Document leveldata.Document
				Package  string
				Variant  string
			}{document, options.Package, options.Variant},
			Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: filesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := leveldata.Encode(document)
				if err != nil {
					return nil, err
				}

				sum := fmt.Sprintf("%x", sha256.Sum256(data))

				file := "levels/" + sum + ".json"
				if options.Variant != "" {
					file = "levels/profiles/" + options.Variant + "/" + sum + ".json"
				}

				code := fmt.Sprintf(
					"// Code generated by Ashura level build. DO NOT EDIT.\npackage %s\nimport \"github.com/karty-game/ashura/pkg/scene/resource\"\nfunc %s() resource.Asset { return resource.Asset{ID:%s,Path:%s,Size:%d,SHA256:%s} }\n",
					options.Package,
					name,
					strconv.Quote(record.Source),
					strconv.Quote(file),
					len(data),
					strconv.Quote(sum),
				)

				formatted, err := format.Source([]byte(code))
				if err != nil {
					return nil, fmt.Errorf("invalid generated level descriptor: %w", err)
				}

				encoded, err := assetbuild.EncodeBundle(
					assetbuild.Bundle{"public/" + file: data, options.Package + "/" + name + ".go": formatted},
				)

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

type invalidLevelPublisher struct{ packageName, variant string }

func (invalidLevelPublisher) Name() string { return levelPublishPluginName }
func (plugin invalidLevelPublisher) Plan(*assetbuild.Plan) error {
	if plugin.variant != "" {
		return fmt.Errorf("invalid generated level descriptor variant %q: %w", plugin.variant, ErrInvalid)
	}

	return fmt.Errorf("invalid level descriptor package %q: %w", plugin.packageName, ErrInvalid)
}
func validLevelIdentifier(name string) bool {
	return name != "_" && token.IsIdentifier(name) && !token.Lookup(name).IsKeyword()
}

func publicationSchema(options PublishOptions) string {
	if options.NativeTextures != nil {
		return leveldata.Schema7
	}

	if options.NativeHDR != nil {
		if options.NativeHDR.NativeAlbedo || nativeHDRFamily(*options.NativeHDR) == nativeHDRMobileFamily {
			return leveldata.Schema5
		}

		return leveldata.Schema4
	}

	if options.AlbedoETC1S != nil {
		return leveldata.Schema3
	}

	return leveldata.Schema
}
