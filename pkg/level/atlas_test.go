//go:build !js

package level

import (
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestLargeRNMAtlasKeepsMaterialLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	data := fixtureTexture(t, assetbuild.UsageLightmap, 2048, [4]byte{1, 2, 3, 125})
	ref := payload(t, root, data)

	verified, err := resource.ReadVerified(
		t.Context(),
		root,
		resource.Asset{ID: "atlas", Path: ref.Path, Size: ref.Size, SHA256: ref.SHA256},
	)
	if err != nil {
		t.Fatal(err)
	}

	prepared := &PreparedChunk{payloads: map[string]*resource.Payload{ref.SHA256: verified},
		images: make(map[string]texture.Data), imageInfo: make(map[string]imageMetadata)}
	if err := prepared.admitImage(ref, texture.Linear, 2048, 2048); err != nil {
		t.Fatal("prepared RNM atlas rejected", err)
	}

	if err := prepared.admitImage(ref, texture.Linear, 0, 0); err == nil {
		t.Fatal("large cached RNM image admitted as a material texture")
	}
}
