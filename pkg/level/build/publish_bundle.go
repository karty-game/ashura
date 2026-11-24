//go:build !js

package levelbuild

import (
	"archive/zip"
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

type publishedChunkBundle struct {
	Bundle leveldata.ChunkBundle
	File   fileRecord
}

func planBundledLevel(plan *assetbuild.Plan, resource assetbuild.Resource, record partitioned,
	document leveldata.Document, files map[string]fileRecord, options PublishOptions) error {
	inputs := []assetbuild.Ref{resource.Ref}
	for _, chunk := range document.Chunks {
		ref, err := planChunkBundle(plan, resource.ID, chunk, files, chunkBundleDirectory(options))
		if err != nil {
			return err
		}

		inputs = append(inputs, ref)
	}

	return plan.Expand(assetbuild.Expansion{
		ID: "level/bundled-files/" + resource.ID, Inputs: inputs,
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			document.Schema = leveldata.Schema8
			published := retainedExternalFiles(document, files)

			for index, ref := range inputs[1:] {
				var bundle publishedChunkBundle
				if err := readRecord(ctx, ref, &bundle); err != nil {
					return err
				}

				document.Chunks[index].Bundle = &bundle.Bundle
				if previous, ok := published[bundle.File.Reference.Path]; ok && previous != bundle.File {
					return ErrInvalid
				}

				published[bundle.File.Reference.Path] = bundle.File
			}

			if err := leveldata.Validate(document); err != nil {
				return err
			}

			if err := publishSharedFiles(child, resource.ID, published, options.sharedFiles); err != nil {
				return err
			}

			return publishLevelManifest(child, resource, record, document, options)
		},
	})
}

func chunkBundleDirectory(options PublishOptions) string {
	family := ""
	if options.NativeTextures != nil {
		family = nativeHDRFamily(nativeTextureAlbedoProfile(*options.NativeTextures))
	} else if options.NativeHDR != nil {
		family = nativeHDRFamily(*options.NativeHDR)
	}

	if family == "" {
		return "levels/chunks"
	}

	return profileLevelDirectory(family) + "/chunks"
}

func profileLevelDirectory(family string) string {
	if family == nativeHDRMobileFamily {
		family = "mobile"
	}

	return "texture-profiles/" + family + "/levels"
}

func planChunkBundle(plan *assetbuild.Plan, level string, chunk leveldata.Chunk, files map[string]fileRecord,
	directory string) (assetbuild.Ref, error) {
	if err := leveldata.ValidateChunk(chunk); err != nil {
		return assetbuild.Ref{}, err
	}

	for _, page := range chunk.Pages {
		if page.Representation != leveldata.RGBDiffuse && page.Representation != leveldata.RNMFit {
			return assetbuild.Ref{}, fmt.Errorf("chunk bundles require calibrated RGB or RNM pages: %w", ErrInvalid)
		}
	}

	owned, err := leveldata.ChunkOwnedReferences(chunk)
	if err != nil {
		return assetbuild.Ref{}, err
	}

	slices.SortFunc(owned, func(a, b leveldata.Ref) int { return strings.Compare(a.SHA256, b.SHA256) })
	records := make([]fileRecord, 0, len(owned))

	artifacts := make([]assetbuild.Artifact, 0, len(owned))
	for _, ref := range owned {
		file, ok := files[ref.Path]
		if !ok || file.Reference != ref || file.Artifact.SHA256 != ref.SHA256 || file.Artifact.Size != ref.Size {
			return assetbuild.Ref{}, ErrInvalid
		}

		records = append(records, file)
		artifacts = append(artifacts, file.Artifact)
	}

	refs, err := plan.Add(assetbuild.Action{
		Parallel: true, ID: "level/chunk-bundle/" + level + "/" + chunk.ID,
		Processor: "level-chunk-bundle", Revision: "stored-zip@2", Options: struct {
			References []leveldata.Ref
			Directory  string
		}{owned, directory}, Artifacts: artifacts,
		Outputs: []assetbuild.Slot{{Name: slot, Schema: "ashura.chunk-bundle@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			return storeChunkBundle(ctx, records, directory)
		},
	})
	if err != nil {
		return assetbuild.Ref{}, err
	}

	return refs[0], nil
}

func storeChunkBundle(ctx *assetbuild.Context, records []fileRecord, directory string) (assetbuild.Result, error) {
	encoded, entries, err := encodeChunkBundle(ctx, records)
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(encoded)
	if err != nil {
		return nil, err
	}

	ref, err := leveldata.Reference(directory+"/"+artifact.SHA256+".zip", encoded)
	if err != nil {
		return nil, err
	}

	file := fileRecord{Reference: ref, Artifact: artifact}

	return result(publishedChunkBundle{Bundle: leveldata.ChunkBundle{File: file.Reference, Entries: entries}, File: file})
}

// Only chunk-owned files are removed. A matching path used by a material,
// developer input or independent model remains an ordinary external resource.
func retainedExternalFiles(document leveldata.Document, files map[string]fileRecord) map[string]fileRecord {
	owned, external := map[string]bool{}, map[string]bool{}

	for _, chunk := range document.Chunks {
		for _, page := range chunk.Pages {
			owned[page.Geometry.Path], owned[page.UV.Path] = true, true
			for _, ref := range page.Maps[:page.MapCount()] {
				owned[ref.Path] = true
			}

			if page.BakeInputs != nil {
				external[page.BakeInputs.Path] = true
			}

			for _, material := range page.Materials {
				if material.Albedo != nil {
					external[material.Albedo.Path] = true
				}

				if material.SSBump != nil {
					external[material.SSBump.Path] = true
				}
			}
		}
	}

	for _, object := range document.Objects {
		if object.Asset != nil {
			external[object.Asset.Path] = true
		}
	}

	retained := make(map[string]fileRecord)

	for path, file := range files {
		if !owned[path] || external[path] {
			retained[path] = file
		}
	}

	return retained
}

const storedZIPEntryOverhead = 238 // 30-byte header, digest name twice, 16-byte descriptor, 46-byte directory entry, two 9-byte timestamps.
const storedZIPEndBytes = 22
const storedZIPUnix1980 = 315532800 // 1980-01-01 00:00:00 UTC.

type boundedChunkZIP struct{ bytes.Buffer }

func (writer *boundedChunkZIP) Write(data []byte) (int, error) {
	if len(data) > leveldata.MaxPayloadBytes-writer.Len() {
		return 0, fmt.Errorf("chunk ZIP exceeds payload limit: %w", ErrInvalid)
	}

	return writer.Buffer.Write(data)
}

func encodeChunkBundle(ctx *assetbuild.Context, records []fileRecord) ([]byte, []leveldata.BundleEntry, error) {
	size, err := chunkZIPCapacity(records)
	if err != nil {
		return nil, nil, err
	}

	output := &boundedChunkZIP{Buffer: *bytes.NewBuffer(make([]byte, 0, size))}
	archive := zip.NewWriter(output)

	for _, file := range records {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		data, err := ctx.ReadArtifact(file.Artifact)
		if err != nil {
			return nil, nil, err
		}

		if len(data) != file.Reference.Size {
			return nil, nil, ErrInvalid
		}

		header := &zip.FileHeader{
			Name: file.Reference.SHA256, Method: zip.Store, Modified: time.Unix(storedZIPUnix1980, 0).UTC(),
		}
		header.SetMode(0644)

		entry, err := archive.CreateHeader(header)
		if err != nil {
			return nil, nil, err
		}

		if _, err = entry.Write(data); err != nil {
			return nil, nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, nil, err
	}

	data := output.Bytes()
	entries, err := indexChunkZIP(data, records)

	return data, entries, err
}

func chunkZIPCapacity(records []fileRecord) (int, error) {
	size := storedZIPEndBytes
	for _, file := range records {
		if file.Reference.Size < 1 || file.Reference.Size > leveldata.MaxPayloadBytes-storedZIPEntryOverhead-size {
			return 0, fmt.Errorf("chunk ZIP exceeds payload limit: %w", ErrInvalid)
		}

		size += storedZIPEntryOverhead + file.Reference.Size
	}

	return size, nil
}

//nolint:gosec // Published references have positive sizes below the admitted 128 MiB payload bound.
func indexChunkZIP(data []byte, records []fileRecord) ([]leveldata.BundleEntry, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	if len(archive.File) != len(records) {
		return nil, ErrInvalid
	}

	entries := make([]leveldata.BundleEntry, len(records))
	for index, file := range archive.File {
		expected := records[index].Reference

		offset, err := file.DataOffset()
		if err != nil {
			return nil, err
		}

		if file.Name != expected.SHA256 || file.Method != zip.Store || file.UncompressedSize64 != uint64(expected.Size) ||
			offset < 0 || offset > int64(len(data))-int64(expected.Size) {
			return nil, ErrInvalid
		}

		entries[index] = leveldata.BundleEntry{SHA256: file.Name, Offset: offset}
	}

	return entries, nil
}
