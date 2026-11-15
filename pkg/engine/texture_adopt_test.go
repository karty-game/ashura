package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestTextureAdoptionRetainsPixelsWithoutAliasingMipDescriptors(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	pixels := []byte{1, 2, 3, 255}
	data := texture.Data{Width: 1, Height: 1, ColorSpace: texture.Linear, Levels: [][]byte{pixels}}

	adopted, err := app.AdoptTexture(data)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := app.NewTexture(data)
	if err != nil {
		t.Fatal(err)
	}

	if &texturedata.Levels(adopted)[0][0] != &pixels[0] || &texturedata.Levels(copied)[0][0] == &pixels[0] {
		t.Fatal("adoption copied pixels or NewTexture retained caller-owned pixels")
	}

	data.Levels[0] = []byte{99, 99, 99, 255}
	if texturedata.Levels(adopted)[0][0] != 1 {
		t.Fatal("caller changed adopted mip descriptors")
	}

	if err := app.UnloadTexture(adopted); err != nil || !texturedata.Valid(copied) {
		t.Fatal("adopted release changed another owned handle", err)
	}
}

func TestTextureAdoptionPreservesAdmissionAndAppLifetime(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	for _, data := range []texture.Data{
		{Width: 1, Height: 1, ColorSpace: texture.Linear},
		{Width: 4097, Height: 1, ColorSpace: texture.Linear, Levels: [][]byte{{1, 2, 3, 4}}},
		{Width: 2, Height: 2, ColorSpace: texture.Linear, Levels: [][]byte{{1, 2, 3, 4}}},
		{Width: 1, Height: 1, ColorSpace: 99, Levels: [][]byte{{1, 2, 3, 4}}},
	} {
		if _, err := app.AdoptTexture(data); err == nil || len(app.textures) != 0 {
			t.Fatal("invalid adoption changed App resource ownership")
		}
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := app.AdoptTexture(texture.Data{}); err == nil {
		t.Fatal("closed App adopted a texture")
	}
}
