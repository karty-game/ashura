package engine

import (
	"testing"

	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

type textureFixture struct{ image *texture.Texture }

func (fixture textureFixture) RuntimeTextures() (*texture.Texture, *texture.Texture, Vector2, float32) {
	return fixture.image, nil, Vector2{X: 1, Y: 1}, 1
}

func TestTextureOwnershipAndUnload(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	pixels := []byte{255, 128, 64, 255}

	image, err := app.NewTexture(texture.Data{Width: 1, Height: 1, ColorSpace: texture.SRGB, Levels: [][]byte{pixels}})
	if err != nil {
		t.Fatal(err)
	}

	pixels[0] = 0

	if texturedata.Levels(image)[0][0] == 0 {
		t.Fatal("texture retained mutable caller bytes")
	}

	foreign := NewApp(AppConfig{})
	defer func() { _ = foreign.Close() }()

	if err := foreign.UnloadTexture(image); err == nil {
		t.Fatal("foreign app unloaded a texture")
	}

	scene := NewWorld()
	entity := scene.Add(textureFixture{image: image})

	app.scene = scene
	if err := app.UnloadTexture(image); err == nil {
		t.Fatal("unloaded a texture still in use")
	}

	scene.Remove(entity)

	if err := app.UnloadTexture(image); err != nil {
		t.Fatal(err)
	}

	if texturedata.Valid(image) {
		t.Fatal("unloaded handle remained valid")
	}

	if err := app.validateTextures(image); err == nil {
		t.Fatal("unloaded material reference was accepted")
	}
}

func TestPreparedTextureMipValidation(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	for _, data := range []texture.Data{
		{},
		{Width: 1, Height: 1, Levels: [][]byte{{0}}},
		{Width: 1, Height: 1, Levels: [][]byte{{0, 0, 0, 255}, {0, 0, 0, 255}}},
	} {
		if _, err := app.NewTexture(data); err == nil {
			t.Fatalf("accepted invalid prepared data: %+v", data)
		}
	}
}

type occlusionFixture struct{ image *texture.Texture }

func (fixture occlusionFixture) RuntimeOcclusion() (*texture.Texture, float32) {
	return fixture.image, 1
}

func TestOcclusionReferencePreventsUnload(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	image, err := app.NewTexture(texture.Data{Width: 1, Height: 1, ColorSpace: texture.Linear, Levels: [][]byte{{255, 255, 255, 255}}})
	if err != nil {
		t.Fatal(err)
	}

	app.scene = NewWorld()

	entity := app.scene.Add(occlusionFixture{image: image})
	if err := app.UnloadTexture(image); err == nil {
		t.Fatal("unloaded an AO texture still referenced by the scene")
	}

	app.scene.Remove(entity)

	if err := app.UnloadTexture(image); err != nil {
		t.Fatal(err)
	}
}
