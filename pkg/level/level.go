// Package level loads externally delivered, build-prepared static level chunks.
// Geometry partitioning, UV generation and lighting integration are build-only.
package level

import (
	"context"
	"fmt"
	"slices"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/resourcedata"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/pkg/texture"
)

type Manifest = leveldata.Document
type Chunk = leveldata.Chunk
type Page = leveldata.Page
type Material = leveldata.Material
type Object = leveldata.Object
type Reference = leveldata.Ref
type ChunkBundle = leveldata.ChunkBundle
type BundleEntry = leveldata.BundleEntry

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
	payloads  map[string]*resource.Payload
	pages     []preparedPage
	images    map[string]texture.Data
	resident  map[string]imageMetadata
	imageInfo map[string]imageMetadata
	imageRefs []Reference
}
type preparedPage struct {
	description Page
	layout      *leveldata.UVLayout
	asset       model.Asset
}

// ReadChunk fetches only one chunk's bounded dependency family on an I/O task.
// Duplicate references share bytes and container metadata. No source asset is
// decoded, geometry transformed, texture repacked or GPU resource created.
func ReadChunk(ctx context.Context, base string, chunk Chunk) (*PreparedChunk, error) {
	if ctx == nil {
		return nil, leveldata.ErrFormat
	}

	request, err := newReadRequest(chunk, texture.RGBA8, nil, nil)
	if err != nil {
		return nil, err
	}

	return request.Read(ctx, base)
}

// Read fetches only dependencies absent from the App-loop residency snapshot.
// Cached images need neither HTTP/file reads nor KTX metadata decoding. Begin
// checks the snapshot against current residency before acquiring any handles.
func (request *ReadRequest) Read(ctx context.Context, base string) (*PreparedChunk, error) {
	if ctx == nil || request == nil || request.refs == nil {
		return nil, leveldata.ErrFormat
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result := &PreparedChunk{
		chunk:     request.chunk,
		payloads:  make(map[string]*resource.Payload, len(request.refs)),
		images:    make(map[string]texture.Data),
		resident:  request.resident,
		imageInfo: make(map[string]imageMetadata),
	}
	if err := result.readFiles(ctx, base, request.refs); err != nil {
		return nil, err
	}

	if request.decoder != nil {
		if err := result.transcodeAlbedos(ctx, request.decoder, request.format); err != nil {
			return nil, fmt.Errorf("chunk %s: %w", request.chunk.ID, err)
		}
	}

	if err := result.admit(); err != nil {
		return nil, fmt.Errorf("chunk %s: %w", request.chunk.ID, err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (prepared *PreparedChunk) payloadBytes(key string) []byte {
	return resourcedata.Bytes(prepared.payloads[key])
}

func cloneChunk(value Chunk) Chunk {
	result := value

	result.Sources = slices.Clone(value.Sources)
	if value.Bundle != nil {
		bundle := *value.Bundle
		bundle.Entries = slices.Clone(value.Bundle.Entries)
		result.Bundle = &bundle
	}

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
