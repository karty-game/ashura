package level

import (
	"errors"

	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

// ErrStaleRequest means a texture omitted from I/O was evicted before the load
// task acquired it. Create a fresh Request and Read; no stale handle is used.
var ErrStaleRequest = errors.New("level texture residency changed; retry the chunk request")

type imageMetadata struct {
	width, height, levels, size int
	space                       texture.ColorSpace
	format                      texture.Format
}

func metadata(data texture.Data, size int) imageMetadata {
	return imageMetadata{
		width:  data.Width,
		height: data.Height,
		levels: len(data.Levels),
		size:   size,
		space:  data.ColorSpace,
		format: data.Format,
	}
}
func (shared *sharedTexture) metadata() imageMetadata {
	return imageMetadata{
		width:  shared.width,
		height: shared.height,
		levels: shared.levels,
		size:   shared.size,
		space:  shared.space,
		format: shared.format,
	}
}

// ReadRequest owns an immutable snapshot of one chunk and its resident image
// metadata. It contains no App handles and is safe to pass to an I/O goroutine.
type ReadRequest struct {
	chunk    Chunk
	refs     []Reference
	resident map[string]imageMetadata
	decoder  basistranscode.Decoder
	format   texture.Format
}

// Request snapshots only the selected chunk's resident texture dependencies on
// the App loop. It creates no resources, pins nothing and performs no I/O.
func (loader *Loader) Request(chunk Chunk) (*ReadRequest, error) {
	if loader == nil || loader.closed || loader.app.Closed() {
		return nil, errLifetime
	}

	request, err := newReadRequest(chunk, loader.app.CompressedAlbedoFormat(), loader.app.SupportsTextureFormat, loader.decoder)
	if err != nil {
		return nil, err
	}

	for _, ref := range request.refs {
		if shared := loader.textures[ref.SHA256]; shared != nil {
			if !texturedata.Valid(shared.image) {
				return nil, errLifetime
			}

			if shared.size != ref.Size {
				return nil, leveldata.ErrFormat
			}

			if request.resident == nil {
				request.resident = make(map[string]imageMetadata)
			}

			request.resident[ref.SHA256] = shared.metadata()
		}
	}

	return request, nil
}

func newReadRequest(
	chunk Chunk,
	format texture.Format,
	supports func(texture.Format) bool,
	decoder basistranscode.Decoder,
) (*ReadRequest, error) {
	refs, err := leveldata.ValidatedChunkReferences(chunk)
	if err != nil {
		return nil, err
	}

	if err := validateDeviceProfile(chunk, format, supports, decoder != nil); err != nil {
		return nil, err
	}

	return &ReadRequest{chunk: cloneChunk(chunk), refs: refs, decoder: decoder, format: format}, nil
}
