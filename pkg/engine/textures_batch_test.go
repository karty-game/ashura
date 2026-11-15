package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func batchTexture(t *testing.T, app *App) *texture.Texture {
	t.Helper()

	image, err := app.NewTexture(texture.Data{
		Width: 1, Height: 1, ColorSpace: texture.SRGB, Levels: [][]byte{{255, 255, 255, 255}},
	})
	if err != nil {
		t.Fatal(err)
	}

	return image
}

func TestUnloadTexturesRejectsWholeBatchBeforeMutation(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	first, second := batchTexture(t, app), batchTexture(t, app)
	app.scene = NewWorld()
	entity := app.scene.Add(textureFixture{image: second})

	if err := app.UnloadTextures([]*texture.Texture{first, second}); err == nil {
		t.Fatal("unloaded a batch with a late referenced texture")
	}

	if !texturedata.Valid(first) || !texturedata.Valid(second) || len(app.textures) != 2 {
		t.Fatal("failed preflight partially unloaded the batch")
	}

	app.scene.Remove(entity)

	if err := app.UnloadTextures([]*texture.Texture{first, nil}); err == nil {
		t.Fatal("accepted an invalid late handle")
	}

	if !texturedata.Valid(first) {
		t.Fatal("invalid late handle released an earlier valid handle")
	}

	if err := app.UnloadTextures([]*texture.Texture{first, second, first}); err != nil {
		t.Fatal(err)
	}

	if texturedata.Valid(first) || texturedata.Valid(second) || len(app.textures) != 0 {
		t.Fatal("successful batch retained resources")
	}
}

func TestUnloadTexturesRejectsForeignAndStaleHandles(t *testing.T) {
	t.Parallel()

	app, foreign := NewApp(AppConfig{}), NewApp(AppConfig{})
	defer func() { _ = app.Close() }()
	defer func() { _ = foreign.Close() }()

	owned, other := batchTexture(t, app), batchTexture(t, foreign)
	if err := app.UnloadTextures([]*texture.Texture{owned, other}); err == nil {
		t.Fatal("accepted a foreign late handle")
	}

	if !texturedata.Valid(owned) || !texturedata.Valid(other) {
		t.Fatal("foreign batch failure released resources")
	}

	if err := app.UnloadTexture(owned); err != nil {
		t.Fatal(err)
	}

	if err := app.UnloadTextures([]*texture.Texture{owned}); err == nil {
		t.Fatal("accepted an unloaded handle")
	}
}

func TestAppClosedAndEmptyTextureBatch(t *testing.T) {
	t.Parallel()

	var absent *App
	if !absent.Closed() || absent.UnloadTextures(nil) == nil {
		t.Fatal("nil App accepted resource work")
	}

	app := NewApp(AppConfig{})
	if app.Closed() {
		t.Fatal("new App reports closed")
	}

	if err := app.UnloadTextures(nil); err != nil {
		t.Fatal(err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if !app.Closed() || app.UnloadTextures(nil) == nil {
		t.Fatal("closed App accepted resource work")
	}
}

type countedTextureFixture struct{ calls *int }

func (fixture countedTextureFixture) RuntimeTextures() (*texture.Texture, *texture.Texture, Vector2, float32) {
	*fixture.calls++

	return nil, nil, Vector2{X: 1, Y: 1}, 1
}

func TestUnloadTexturesScansSceneOnce(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	images := []*texture.Texture{batchTexture(t, app), batchTexture(t, app), batchTexture(t, app)}
	calls := 0
	app.scene = NewWorld()
	app.scene.Add(countedTextureFixture{calls: &calls})

	if err := app.UnloadTextures(images); err != nil {
		t.Fatal(err)
	}

	if calls != 1 {
		t.Fatalf("batch unload scanned one scene provider %d times, want once", calls)
	}
}
