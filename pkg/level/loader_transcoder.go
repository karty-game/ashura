package level

import (
	"context"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/texture"
)

// AlbedoTranscoder is an explicit compatibility dependency for ETC1S albedos.
// Native GPU payloads do not use it. Implementations retain the previously
// approved loading-only contract: return compressed GPU blocks, not RGBA pixels.
type AlbedoTranscoder interface {
	Transcode(ctx context.Context, encoded []byte, format texture.Format) (texture.Data, error)
	Close()
}

// NewLoaderWithTranscoder opts into legacy ETC1S albedo loading. The loader
// takes ownership of transcoder and closes it with its App-bound lifetime,
// including when app is already closed. Native payloads bypass transcoding.
// Passing nil selects the same decoder-free behavior as NewLoader.
func NewLoaderWithTranscoder(app *engine.App, transcoder AlbedoTranscoder) *Loader {
	loader := &Loader{
		app:      app,
		textures: make(map[string]*sharedTexture),
		chunks:   make(map[*LoadedChunk]bool),
		tasks:    make(map[*LoadTask]bool),
		decoder:  transcoder,
	}
	if app.Closed() {
		if loader.decoder != nil {
			loader.decoder.Close()
		}

		loader.closed = true

		return loader
	}

	var err error

	loader.cancel, err = app.OnClose(func() { _ = loader.Close() })
	if err != nil {
		if loader.decoder != nil {
			loader.decoder.Close()
		}

		loader.closed = true
	}

	return loader
}
