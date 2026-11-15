package level

import (
	"context"
	"errors"

	"github.com/karty-game/karty-webgpu/internal/basistranscode"
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
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

	if err := leveldata.ValidateChunk(chunk); err != nil {
		return nil, err
	}

	format := loader.app.CompressedAlbedoFormat()
	if err := validateDeviceProfile(chunk, format, loader.decoder != nil); err != nil {
		return nil, err
	}

	refs, err := leveldata.ChunkReferences(chunk)
	if err != nil {
		return nil, err
	}

	request := &ReadRequest{
		chunk:    cloneChunk(chunk),
		resident: make(map[string]imageMetadata),
		decoder:  loader.decoder,
		format:   format,
	}

	for _, ref := range refs {
		if shared := loader.textures[ref.SHA256]; shared != nil {
			if !texturedata.Valid(shared.image) {
				return nil, errLifetime
			}

			if shared.size != ref.Size {
				return nil, leveldata.ErrFormat
			}

			request.resident[ref.SHA256] = shared.metadata()
		}
	}

	return request, nil
}

// Read fetches only dependencies absent from the App-loop residency snapshot.
// Cached images need neither HTTP/file reads nor KTX metadata decoding. Begin
// checks the snapshot against current residency before acquiring any handles.
func (request *ReadRequest) Read(ctx context.Context, base string) (*PreparedChunk, error) {
	if request == nil {
		return nil, leveldata.ErrFormat
	}

	return readChunk(ctx, base, request.chunk, request.resident, request.decoder, request.format)
}
