//go:build !js

package assetbuild

import "testing"

func TestAtlasRecordsRequireExplicitMipCap(t *testing.T) {
	t.Parallel()

	value := Image{
		Name: "Page", Source: "atlas/page", Usage: UsageAlbedo,
		Width: 1, Height: 1, Levels: [][]byte{{255, 255, 255, 255}}, Atlas: true,
	}
	if _, err := EncodeImageSet(ImageSet{Images: []Image{value}}); err == nil {
		t.Fatal("accepted atlas without an explicit safe mip cap")
	}

	value.MaxMipLevels = 1
	if _, err := EncodeImageSet(ImageSet{Images: []Image{value}}); err != nil {
		t.Fatal(err)
	}
}
