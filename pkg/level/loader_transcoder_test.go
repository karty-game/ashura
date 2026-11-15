//go:build !js

package level

import (
	"context"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

var errUnexpectedTranscode = errors.New("native payload unexpectedly requested runtime transcoding")

type observingTranscoder struct {
	calls, closes int
}

func (transcoder *observingTranscoder) Transcode(context.Context, []byte, texture.Format) (texture.Data, error) {
	transcoder.calls++

	return texture.Data{}, errUnexpectedTranscode
}

func (transcoder *observingTranscoder) Close() { transcoder.closes++ }

func TestNativePayloadsBypassOptionalTranscoderAndKeepOwnedLifetime(t *testing.T) {
	t.Parallel()

	for _, owner := range []string{"loader", "app"} {
		t.Run(owner, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()

			prepared, err := ReadChunk(t.Context(), root, fixtureChunk(t, root))
			if err != nil {
				t.Fatal(err)
			}

			app := engine.NewApp(engine.AppConfig{})

			t.Cleanup(func() { _ = app.Close() })

			transcoder := &observingTranscoder{}

			loader := NewLoaderWithTranscoder(app, transcoder)
			if _, err = loader.Load(prepared); err != nil {
				t.Fatal(err)
			}

			if transcoder.calls != 0 || transcoder.closes != 0 {
				t.Fatal("prepared native textures must bypass the optional decoder")
			}

			if owner == "app" {
				if err = app.Close(); err != nil {
					t.Fatal(err)
				}
			}

			if err = loader.Close(); err != nil {
				t.Fatal(err)
			}

			if err = app.Close(); err != nil {
				t.Fatal(err)
			}

			if err = loader.Close(); err != nil || transcoder.calls != 0 || transcoder.closes != 1 {
				t.Fatalf(
					"transcoder lifetime was not owned exactly once: calls=%d closes=%d error=%v",
					transcoder.calls, transcoder.closes, err,
				)
			}
		})
	}
}

func TestClosedAppDefaultLoaderNeedsNoDecoderAndClosesExplicitDecoder(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	plain := NewLoader(app)
	if err := plain.Close(); err != nil {
		t.Fatal(err)
	}

	transcoder := &observingTranscoder{}

	explicit := NewLoaderWithTranscoder(app, transcoder)
	if err := explicit.Close(); err != nil || transcoder.calls != 0 || transcoder.closes != 1 {
		t.Fatalf(
			"closed App must release supplied decoder exactly once: calls=%d closes=%d error=%v",
			transcoder.calls, transcoder.closes, err,
		)
	}
}
