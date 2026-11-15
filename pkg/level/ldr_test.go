//go:build !js

package level

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestLDRProfileUsesExactFormatAndHardwareTransfer(t *testing.T) {
	t.Parallel()

	for _, profile := range []struct {
		storage string
		format  texture.Format
		device  texture.Format
	}{
		{leveldata.StorageBC1LDR, texture.BC1, texture.BC1},
		{leveldata.StorageBC7LDR, texture.BC7, texture.BC1},
		{leveldata.StorageETC2LDR, texture.ETC2RGB, texture.ETC2RGB},
	} {
		t.Run(profile.storage, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			chunk := fixtureChunk(t, root)
			page := &chunk.Pages[0]
			page.Storage, page.Encoding, page.Representation = profile.storage, leveldata.EncodingSRGB, RNMFit

			format, space := lightmapImageProfile(*page)
			if format != profile.format || space != texture.SRGB {
				t.Fatal("wrong LDR sampling profile", format, space)
			}

			page.Encoding = "linear"

			_, space = lightmapImageProfile(*page)
			if space != texture.Linear {
				t.Fatal("linear payload silently acquired sRGB transfer")
			}

			page.Encoding = leveldata.EncodingSRGB

			missing := filepath.Join(root, "missing-payload-root")
			for _, device := range []texture.Format{texture.RGBA8, texture.BC1, texture.ETC2RGB} {
				prepared, err := readChunk(t.Context(), missing, chunk, nil, nil, device)
				if device == profile.device {
					if prepared != nil || !errors.Is(err, fs.ErrNotExist) {
						t.Fatal("supported profile did not reach payload I/O", err)
					}
				} else if prepared != nil || !errors.Is(err, ErrNativeTextures) {
					t.Fatal("unsupported profile reached payload I/O", err)
				}
			}

			prepared, err := readChunk(t.Context(), root, chunk, nil, nil, profile.device)
			if prepared != nil || !errors.Is(err, leveldata.ErrFormat) {
				t.Fatal("raw RGBE payload admitted as native sRGB LDR", err)
			}
		})
	}
}

func TestLDRMobileReliefNeedsNoOptionalCompression(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	data := fixtureTexture(t, "ssbump", 4, [4]byte{41, 97, 155, 255})
	ref := payload(t, root, data)
	material := Material{SSBump: &ref}

	prepared := &PreparedChunk{files: map[string][]byte{ref.SHA256: data},
		images: make(map[string]texture.Data), imageInfo: make(map[string]imageMetadata)}
	if err := prepared.admitMaterialImages(material, leveldata.StorageETC2LDR); err != nil {
		t.Fatal("exact portable relief rejected", err)
	}

	for _, format := range []texture.Format{texture.ETC2RGB, texture.ASTC4x4, texture.BC7} {
		info := prepared.imageInfo[ref.SHA256]
		info.format = format

		prepared.imageInfo[ref.SHA256] = info
		if err := prepared.admitMaterialImages(material, leveldata.StorageETC2LDR); !errors.Is(err, leveldata.ErrFormat) {
			t.Fatal("mobile LDR silently accepted lossy or optional relief format", format, err)
		}
	}
}
