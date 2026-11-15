//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// NativeHDROptions selects offline GPU-native RNM and SSBump encoding.
// Embedded portable libraries prepare GPU blocks offline. No codec processing occurs in the game.
type NativeHDROptions struct {
	Encoder assetbuild.BlockCompressionOptions

	// Family selects "bc" (also the empty default) or "etc2-astc".
	// BC uses BC6H RNM and BC7 SSBump; the mobile family uses RGBE ETC2
	// RNM and linear ASTC SSBump with the existing RGBE shader path.
	Family string
	// NativeAlbedo prepares opaque albedos for this family offline, replacing
	// the optional legacy ETC1S loading path with direct GPU block uploads.
	NativeAlbedo bool
}

const (
	nativeBlockProcessor  = "level-native-block"
	nativeBlockFileSchema = "karty.native-block-file@1"
	maxNativeHDREffort    = 4
	nativeRNMRole         = "rnm"
	nativeSSBumpRole      = "ssbump"
	nativeHDRBCFamily     = "bc"
	nativeHDRMobileFamily = "etc2-astc"
)

type nativeBlockInputs struct {
	pages     [][]publishedPage
	files     map[string]fileRecord
	lightmaps map[string]bool
}

func discoverEncodedNativeHDR(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	chunks []assetbuild.Ref,
	options PublishOptions,
) error {
	if options.NativeHDR == nil {
		return discoverAlbedoCompression(ctx, plan, resource, record, chunks, options)
	}

	profile := *options.NativeHDR
	if err := validateNativeHDR(profile); err != nil {
		return err
	}

	inputs, err := collectNativeBlocks(ctx, chunks)
	if err != nil {
		return err
	}

	encoded, dependencies, err := nativeBlockActions(plan, resource.ID, inputs, profile, options.sharedCodecs)
	if err != nil {
		return err
	}

	rewritten, err := nativeIndexActions(plan, resource.ID, chunks, inputs.pages, encoded, dependencies, nativeHDRFamily(profile))
	if err != nil {
		return err
	}

	return plan.Expand(assetbuild.Expansion{
		ID:     "level/native-publish/" + resource.ID,
		Inputs: append([]assetbuild.Ref{resource.Ref}, rewritten...),
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			return discoverAlbedoCompression(ctx, child, resource, record, rewritten, options)
		},
	})
}

func validateNativeHDR(profile NativeHDROptions) error {
	for _, quality := range []*int{profile.Encoder.SSBumpQuality, profile.Encoder.RGBEQuality} {
		if quality != nil && (*quality < 0 || *quality > maxNativeHDREffort) {
			return fmt.Errorf("invalid native UASTC quality: %w", ErrInvalid)
		}
	}

	family := nativeHDRFamily(profile)

	encoder := profile.Encoder
	if family != nativeHDRBCFamily && family != nativeHDRMobileFamily ||
		encoder.Timeout < 0 {
		return fmt.Errorf("invalid native HDR encoder options: %w", ErrInvalid)
	}

	if family == nativeHDRBCFamily && (encoder.HDREffort < 0 || encoder.HDREffort > maxNativeHDREffort) {
		return fmt.Errorf("invalid BC HDR encoder options: %w", ErrInvalid)
	}

	return nil
}

func nativeHDRFamily(profile NativeHDROptions) string {
	if profile.Family == "" {
		return nativeHDRBCFamily
	}

	return profile.Family
}

func collectNativeBlocks(ctx *assetbuild.Context, chunks []assetbuild.Ref) (nativeBlockInputs, error) {
	inputs := nativeBlockInputs{files: map[string]fileRecord{}, lightmaps: map[string]bool{}}

	for _, source := range chunks {
		var pages []publishedPage
		if err := readRecord(ctx, source, &pages); err != nil {
			return nativeBlockInputs{}, err
		}

		inputs.pages = append(inputs.pages, pages)
		for _, page := range pages {
			collectNativePageFiles(page, inputs.files, inputs.lightmaps)
		}
	}

	return inputs, nil
}

func collectNativePageFiles(page publishedPage, files map[string]fileRecord, lightmaps map[string]bool) {
	wanted := map[string]bool{}

	for _, ref := range page.Page.Maps[:page.Page.MapCount()] {
		key := nativeBlockKey(nativeRNMRole, ref.SHA256)
		wanted[key] = true
		lightmaps[key] = true
	}

	for _, material := range page.Page.Materials {
		if material.SSBump != nil {
			wanted[nativeBlockKey(nativeSSBumpRole, material.SSBump.SHA256)] = true
		}
	}

	for _, file := range page.Files {
		for _, role := range []string{nativeRNMRole, nativeSSBumpRole} {
			key := nativeBlockKey(role, file.Reference.SHA256)
			if wanted[key] {
				files[key] = file
			}
		}
	}
}

func nativeBlockKey(role, digest string) string {
	return role + "/" + digest
}

func nativeBlockActions(
	plan *assetbuild.Plan,
	identity string,
	inputs nativeBlockInputs,
	profile NativeHDROptions,
	shared *codecActionCache,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	encoded := map[string]assetbuild.Ref{}

	var dependencies []assetbuild.Ref

	for _, key := range sortedAlbedoKeys(inputs.files) {
		ref, err := addNativeBlockAction(plan, identity, key, inputs.files[key], profile, inputs.lightmaps[key], shared)
		if err != nil {
			return nil, nil, err
		}

		encoded[key] = ref
		dependencies = append(dependencies, ref)
	}

	return encoded, dependencies, nil
}

func addNativeBlockAction(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	profile NativeHDROptions,
	hdr bool,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	if !hdr {
		return addSSBumpNativeAction(plan, identity, key, file, profile, shared)
	}

	refs, err := shared.add(plan, assetbuild.Action{
		ID:        "level/native-block/" + identity + "/" + key,
		Processor: nativeBlockProcessor,
		Revision:  "native-rnm-relief-encoder-quality@3-wasi1",
		Artifacts: []assetbuild.Artifact{file.Artifact},
		Options: struct {
			Profile NativeHDROptions

			HDR bool
		}{profile, hdr},
		Outputs: []assetbuild.Slot{{Name: slot, Schema: nativeBlockFileSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return encodeNativeBlock(ctx, file, profile, hdr)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func encodeNativeBlock(
	ctx *assetbuild.Context,
	file fileRecord,
	profile NativeHDROptions,
	hdr bool,
) (assetbuild.Result, error) {
	family := nativeHDRFamily(profile)

	data, err := ctx.ReadArtifact(file.Artifact)
	if err != nil {
		return nil, err
	}

	directory := "textures"

	data, err = compressNativePayload(ctx, data, profile.Encoder, family, hdr)
	if hdr {
		directory = "lightmaps"
	}

	if err != nil {
		return nil, err
	}

	output, err := storePublished(ctx, directory, ".ktx2", data)
	if err != nil {
		return nil, err
	}

	return result(output)
}

func compressNativePayload(
	ctx *assetbuild.Context,
	data []byte,
	encoder assetbuild.BlockCompressionOptions,
	family string,
	hdr bool,
) ([]byte, error) {
	if family == nativeHDRMobileFamily {
		if hdr {
			return assetbuild.CompressRGBEETC2(ctx, data, encoder)
		}

		return assetbuild.CompressSSBumpASTC(ctx, data, encoder)
	}

	if hdr {
		return assetbuild.CompressRGBERNM(ctx, data, encoder)
	}

	return assetbuild.CompressSSBumpBC7(ctx, data, encoder)
}

func nativeIndexActions(
	plan *assetbuild.Plan,
	identity string,
	chunks []assetbuild.Ref,
	pages [][]publishedPage,
	encoded map[string]assetbuild.Ref,
	dependencies []assetbuild.Ref,
	family string,
) ([]assetbuild.Ref, error) {
	var rewritten []assetbuild.Ref

	for index, source := range chunks {
		current := pages[index]

		refs, err := plan.Add(assetbuild.Action{
			ID:        fmt.Sprintf("level/native-index/%s/%d", identity, index),
			Processor: "level-native-index",
			Revision:  "native-hdr-family@2-wasi1",
			Options:   family,
			Inputs:    append([]assetbuild.Ref{source}, dependencies...),
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return rewriteNativeBlocks(ctx, current, encoded, family)
			},
		})
		if err != nil {
			return nil, err
		}

		rewritten = append(rewritten, refs[0])
	}

	return rewritten, nil
}

func rewriteNativeBlocks(
	ctx *assetbuild.Context,
	pages []publishedPage,
	encoded map[string]assetbuild.Ref,
	family string,
) (assetbuild.Result, error) {
	for index := range pages {
		if err := rewriteNativePage(ctx, &pages[index], encoded, family); err != nil {
			return nil, err
		}
	}

	return result(pages)
}

func rewriteNativePage(ctx *assetbuild.Context, page *publishedPage, encoded map[string]assetbuild.Ref, family string) error {
	if err := replaceNativePageFiles(ctx, page, encoded); err != nil {
		return err
	}

	page.Page.Encoding = "linear"

	page.Page.Storage = "bc-hdr"
	if family == nativeHDRMobileFamily {
		page.Page.Encoding = rgbeEncoding
		page.Page.Storage = nativeHDRMobileFamily
	}

	return nil
}

func replaceNativePageFiles(ctx *assetbuild.Context, page *publishedPage, encoded map[string]assetbuild.Ref) error {
	originalFileCount := len(page.Files)
	replaced := map[string]bool{}

	for index := range page.Page.Maps[:page.Page.MapCount()] {
		file, err := replaceNativeFile(
			ctx, page, nativeRNMRole, page.Page.Maps[index].SHA256, encoded, replaced,
		)
		if err != nil {
			return err
		}

		page.Page.Maps[index] = file.Reference
	}

	for index := range page.Page.Materials {
		material := &page.Page.Materials[index]
		if material.SSBump == nil {
			continue
		}

		file, err := replaceNativeFile(ctx, page, nativeSSBumpRole, material.SSBump.SHA256, encoded, replaced)
		if err != nil {
			return err
		}

		material.SSBump = &file.Reference
	}

	// A canonical replacement may be byte-identical to its source. Remove only
	// old entries; every newly referenced output must remain available.
	page.Files = append(withoutPublishedFiles(page.Files[:originalFileCount], replaced), page.Files[originalFileCount:]...)

	return nil
}

func replaceNativeFile(
	ctx *assetbuild.Context,
	page *publishedPage,
	role, key string,
	encoded map[string]assetbuild.Ref,
	replaced map[string]bool,
) (fileRecord, error) {
	ref, ok := encoded[nativeBlockKey(role, key)]
	if !ok {
		return fileRecord{}, ErrInvalid
	}

	var file fileRecord
	if err := readRecord(ctx, ref, &file); err != nil {
		return fileRecord{}, err
	}

	replaced[key] = true

	page.Files = append(page.Files, file)

	return file, nil
}

func withoutPublishedFiles(files []fileRecord, excluded map[string]bool) []fileRecord {
	keep := make([]fileRecord, 0, len(files))
	for _, file := range files {
		if !excluded[file.Reference.SHA256] {
			keep = append(keep, file)
		}
	}

	return keep
}
