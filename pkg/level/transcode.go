package level

import (
	"context"

	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/pkg/texture"
)

// Transcoding runs on the I/O path in a dedicated web worker. The App loop
// receives immutable GPU blocks and never performs this operation in Step.
func (prepared *PreparedChunk) transcodeAlbedos(ctx context.Context, decoder basistranscode.Decoder, format texture.Format) error {
	for _, page := range prepared.chunk.Pages {
		for _, mat := range page.Materials {
			if mat.Albedo == nil || mat.AlbedoEncoding != etc1sEncoding {
				continue
			}

			key := mat.Albedo.SHA256
			if _, resident := prepared.resident[key]; resident {
				continue
			}

			if _, done := prepared.images[key]; done {
				continue
			}

			if decoder == nil || format == texture.RGBA8 {
				return basistranscode.ErrUnsupported
			}

			image, err := decoder.Transcode(ctx, prepared.payloadBytes(key), format)
			if err != nil {
				return err
			}

			prepared.images[key] = image
		}
	}

	return nil
}
