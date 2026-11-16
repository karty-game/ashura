// Package level loads externally delivered, build-prepared static level chunks.
// Geometry partitioning, UV generation and lighting integration are build-only.
package level

import (
	"context"
	"fmt"
	"slices"

	"github.com/karty-game/karty-webgpu/internal/basistranscode"
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/model"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type Manifest = leveldata.Document
type Chunk = leveldata.Chunk
type Page = leveldata.Page
type Material = leveldata.Material
type Object = leveldata.Object
type Reference = leveldata.Ref

const (
	RGBDiffuse = leveldata.RGBDiffuse
	RNMFit     = leveldata.RNMFit
)

// Read retrieves and admits a bounded level index without loading chunk payloads.
func Read(ctx context.Context, base string, asset resource.Asset) (Manifest, error) {
	if asset.Size < 1 || asset.Size > leveldata.MaxManifestBytes {
		return Manifest{}, leveldata.ErrFormat
	}

	data, err := resource.Read(ctx, base, asset)
	if err != nil {
		return Manifest{}, err
	}

	return leveldata.Decode(data)
}

// PreparedChunk holds one chunk's verified GPU-ready files and admitted metadata.
// It owns no App resources and can be passed from an I/O task to the App loop.
type PreparedChunk struct {
	chunk     Chunk
	files     map[string][]byte
	verified  map[string]*resource.Payload
	pages     []preparedPage
	images    map[string]texture.Data
	resident  map[string]imageMetadata
	imageInfo map[string]imageMetadata
	imageRefs []Reference
}
type preparedPage struct {
	description Page
	layout      *leveldata.UVLayout
	geometry    []byte
	asset       model.Asset
}

// ReadChunk fetches only one chunk's bounded dependency family on an I/O task.
// Duplicate references share bytes and container metadata. No source asset is
// decoded, geometry transformed, texture repacked or GPU resource created.
func ReadChunk(ctx context.Context, base string, chunk Chunk) (*PreparedChunk, error) {
	return readChunk(ctx, base, chunk, nil, nil, texture.RGBA8)
}

func readChunk(
	ctx context.Context,
	base string,
	chunk Chunk,
	resident map[string]imageMetadata,
	decoder basistranscode.Decoder,
	format texture.Format,
) (*PreparedChunk, error) {
	if ctx == nil {
		return nil, leveldata.ErrFormat
	}

	if err := leveldata.ValidateChunk(chunk); err != nil {
		return nil, err
	}

	if err := validateDeviceProfile(chunk, format, decoder != nil); err != nil {
		return nil, err
	}

	refs, err := leveldata.ChunkReferences(chunk)
	if err != nil {
		return nil, err
	}

	result := &PreparedChunk{
		chunk:     cloneChunk(chunk),
		files:     make(map[string][]byte),
		verified:  make(map[string]*resource.Payload),
		images:    make(map[string]texture.Data),
		resident:  resident,
		imageInfo: make(map[string]imageMetadata),
	}
	if err = result.readFiles(ctx, base, refs); err != nil {
		return nil, err
	}

	if decoder != nil {
		if err = result.transcodeAlbedos(ctx, decoder, format); err != nil {
			return nil, fmt.Errorf("chunk %s: %w", chunk.ID, err)
		}
	}

	if err = result.admit(); err != nil {
		return nil, fmt.Errorf("chunk %s: %w", chunk.ID, err)
	}

	if err = ctx.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func cloneChunk(value Chunk) Chunk {
	result := value
	result.Sources = slices.Clone(value.Sources)

	result.Pages = slices.Clone(value.Pages)
	for index := range result.Pages {
		if value.Pages[index].BakeInputs != nil {
			copied := *value.Pages[index].BakeInputs
			result.Pages[index].BakeInputs = &copied
		}

		result.Pages[index].Materials = slices.Clone(value.Pages[index].Materials)
		for material := range result.Pages[index].Materials {
			source := &result.Pages[index].Materials[material]
			if source.Albedo != nil {
				copied := *source.Albedo
				source.Albedo = &copied
			}

			if source.SSBump != nil {
				copied := *source.SSBump
				source.SSBump = &copied
			}
		}
	}

	return result
}
