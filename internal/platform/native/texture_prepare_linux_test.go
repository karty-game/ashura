//go:build linux

package native

import (
	"testing"

	"github.com/karty-game/ashura/internal/texturedata"
)

func TestPrepareTextureReusesCachedGPUImage(t *testing.T) {
	t.Parallel()

	image, err := texturedata.Create(texturedata.Data{
		Width: 1, Height: 1, ColorSpace: texturedata.Linear, Levels: [][]byte{{255, 255, 255, 255}},
	})
	if err != nil {
		t.Fatal(err)
	}

	defer texturedata.Release(image)

	// A nil device is deliberate: the cached path must never allocate or upload.
	driver := &Driver{textures: map[*texturedata.Texture]gpuTexture{image: {bytes: 4}}, textureBytes: 4}
	for range 2 {
		if err := driver.PrepareTexture(image); err != nil {
			t.Fatal(err)
		}
	}

	if len(driver.textures) != 1 || driver.textureBytes != 4 {
		t.Fatal("cached preparation changed texture residency")
	}
}
