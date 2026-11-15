//go:build !js

package levelbuild

import (
	"fmt"
	"sort"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// ETC1SOptions configures offline portable Basis encoding at quality 1..255.
type ETC1SOptions struct {
	Quality int
}

func discoverAlbedoCompression(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	record partitioned,
	chunks []assetbuild.Ref,
	options PublishOptions,
) error {
	if options.AlbedoETC1S == nil {
		return discoverNativeAlbedos(ctx, plan, resource, record, chunks, options)
	}

	profile := *options.AlbedoETC1S
	if profile.Quality < 1 || profile.Quality > 255 {
		return fmt.Errorf("invalid ETC1S encoder options: %w", ErrInvalid)
	}

	inputs, files, err := collectAlbedos(ctx, chunks)
	if err != nil {
		return err
	}

	encoded, encodedRefs, err := encodedAlbedoActions(
		plan,
		resource.ID,
		files,
		profile,
		options.sharedCodecs,
		options.NativeTextures != nil,
	)
	if err != nil {
		return err
	}

	var rewritten []assetbuild.Ref

	for index, source := range chunks {
		pages := inputs[index]

		refs, err := plan.Add(
			assetbuild.Action{
				ID:        fmt.Sprintf("level/etc1s-index/%s/%d", resource.ID, index),
				Processor: "level-etc1s-index",
				Revision:  "replace-albedos@1-wasi1",
				Inputs:    append([]assetbuild.Ref{source}, encodedRefs...),
				Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
				Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
					return rewriteAlbedos(ctx, pages, encoded)
				},
			},
		)
		if err != nil {
			return err
		}

		rewritten = append(rewritten, refs[0])
	}

	return plan.Expand(
		assetbuild.Expansion{
			ID:     "level/etc1s-publish/" + resource.ID,
			Inputs: append([]assetbuild.Ref{resource.Ref}, rewritten...),
			Run: func(ctx *assetbuild.Context, plan *assetbuild.Plan) error {
				return discoverNativeAlbedos(ctx, plan, resource, record, rewritten, options)
			},
		},
	)
}

func encodeETC1S(ctx *assetbuild.Context, data []byte, options ETC1SOptions) ([]byte, error) {
	return assetbuild.PrepareETC1S(ctx, data, options.Quality)
}

func collectAlbedos(ctx *assetbuild.Context, chunks []assetbuild.Ref) ([][]publishedPage, map[string]fileRecord, error) {
	var inputs [][]publishedPage

	files := map[string]fileRecord{}

	for _, ref := range chunks {
		var pages []publishedPage
		if err := readRecord(ctx, ref, &pages); err != nil {
			return nil, nil, err
		}

		inputs = append(inputs, pages)
		for _, p := range pages {
			for _, m := range p.Page.Materials {
				if m.Albedo != nil {
					for _, f := range p.Files {
						if f.Reference == *m.Albedo {
							files[f.Reference.SHA256] = f
						}
					}
				}
			}
		}
	}

	return inputs, files, nil
}

func rewriteAlbedos(ctx *assetbuild.Context, pages []publishedPage, encoded map[string]assetbuild.Ref) (assetbuild.Result, error) {
	return rewriteAlbedoEncoding(ctx, pages, encoded, "etc1s")
}

func rewriteAlbedoEncoding(
	ctx *assetbuild.Context,
	pages []publishedPage,
	encoded map[string]assetbuild.Ref,
	encoding string,
) (assetbuild.Result, error) {
	for i := range pages {
		page := &pages[i]
		replaced := map[string]bool{}

		for j := range page.Page.Materials {
			mat := &page.Page.Materials[j]
			if mat.Albedo == nil {
				continue
			}

			var file fileRecord
			if err := readRecord(ctx, encoded[mat.Albedo.SHA256], &file); err != nil {
				return nil, err
			}

			replaced[mat.Albedo.SHA256] = true
			mat.Albedo = &file.Reference
			mat.AlbedoEncoding = encoding

			page.Files = append(page.Files, file)
		}

		var keep []fileRecord

		for _, file := range page.Files {
			if !replaced[file.Reference.SHA256] {
				keep = append(keep, file)
			}
		}

		page.Files = keep
	}

	return result(pages)
}

func addEncodedAlbedo(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	profile ETC1SOptions,
	shared *codecActionCache,
) (assetbuild.Ref, error) {
	return addEncodedAlbedoWithExecution(plan, identity, key, file, profile, shared, false)
}

func addEncodedAlbedoWithExecution(
	plan *assetbuild.Plan,
	identity, key string,
	file fileRecord,
	profile ETC1SOptions,
	shared *codecActionCache,
	parallel bool,
) (assetbuild.Ref, error) {
	refs, err := shared.add(plan,
		assetbuild.Action{
			Parallel:  parallel,
			ID:        "level/etc1s/" + identity + "/" + key,
			Processor: "level-etc1s",
			Revision:  "opaque-srgb-full-mips@1-wasi1",
			Artifacts: []assetbuild.Artifact{file.Artifact},
			Options:   profile,
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.etc1s-file@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := ctx.ReadArtifact(file.Artifact)
				if err != nil {
					return nil, err
				}

				data, err = encodeETC1S(ctx, data, profile)
				if err != nil {
					return nil, err
				}

				file, err := storePublished(ctx, "textures", ".ktx2", data)
				if err != nil {
					return nil, err
				}

				return result(file)
			},
		},
	)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func sortedAlbedoKeys[T any](files map[string]T) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func encodedAlbedoActions(
	plan *assetbuild.Plan,
	identity string,
	files map[string]fileRecord,
	profile ETC1SOptions,
	shared *codecActionCache,
	parallel bool,
) (map[string]assetbuild.Ref, []assetbuild.Ref, error) {
	encoded := map[string]assetbuild.Ref{}

	var refs []assetbuild.Ref

	for _, key := range sortedAlbedoKeys(files) {
		ref, err := addEncodedAlbedoWithExecution(plan, identity, key, files[key], profile, shared, parallel)
		if err != nil {
			return nil, nil, err
		}

		encoded[key] = ref
		refs = append(refs, ref)
	}

	return encoded, refs, nil
}
