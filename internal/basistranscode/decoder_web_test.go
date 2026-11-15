//go:build js && wasm

package basistranscode

import (
	"context"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestWorkerCompressedBlocksAndLifetime(t *testing.T) {
	t.Parallel()

	decoder := New()
	defer decoder.Close()

	for _, format := range []texturedata.Format{texturedata.BC1, texturedata.ETC2RGB} {
		data, err := decoder.Transcode(context.Background(), testAlbedo, format)
		if err != nil {
			t.Fatal(err)
		}

		image, err := texturedata.FromOwned(data)
		if err != nil {
			t.Fatal(err)
		}

		if texturedata.ByteSize(image) != 56 || texturedata.Encoding(image) != format {
			t.Fatalf("not compressed blocks: %d, %d", texturedata.ByteSize(image), texturedata.Encoding(image))
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := decoder.Transcode(ctx, testAlbedo, texturedata.BC1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := decoder.Transcode(context.Background(), testAlbedo, texturedata.BC1); err != nil {
		t.Fatalf("reuse after cancellation: %v", err)
	}

	decoder.Close()

	if _, err := decoder.Transcode(context.Background(), testAlbedo, texturedata.BC1); !errors.Is(err, context.Canceled) {
		t.Fatalf("close: %v", err)
	}
}
