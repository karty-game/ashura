//go:build !js

package level

import (
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestLargeRNMAtlasKeepsMaterialLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data := fixtureTexture(t, assetbuild.UsageLightmap, 2048, [4]byte{1, 2, 3, 125})
	ref := payload(t, root, data)

	prepared := &PreparedChunk{files: map[string][]byte{ref.SHA256: data},
		images: make(map[string]texture.Data), imageInfo: make(map[string]imageMetadata)}
	if err := prepared.admitImage(ref, texture.Linear, 2048, 2048); err != nil {
		t.Fatal("prepared RNM atlas rejected", err)
	}

	if err := prepared.admitImage(ref, texture.Linear, 0, 0); err == nil {
		t.Fatal("large cached RNM image admitted as a material texture")
	}
}
