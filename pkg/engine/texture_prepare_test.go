package engine

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/texturedata"
)

var errTexturePrepareFixture = errors.New("fixture upload failure")

type texturePrepareDriver struct {
	platform.Driver

	err    error
	images []*texturedata.Texture
}

func (driver *texturePrepareDriver) PrepareTexture(image *texturedata.Texture) error {
	driver.images = append(driver.images, image)

	return driver.err
}

func TestPrepareTextureRejectsInvalidOwnershipBeforeBackend(t *testing.T) {
	t.Parallel()

	app, foreign := NewApp(AppConfig{}), NewApp(AppConfig{})
	defer func() { _ = app.Close() }()
	defer func() { _ = foreign.Close() }()

	owned, other := batchTexture(t, app), batchTexture(t, foreign)
	if err := app.UnloadTexture(owned); err != nil {
		t.Fatal(err)
	}

	driver := &texturePrepareDriver{}

	app.driver = driver
	defer func() { app.driver = nil }()

	for _, image := range []*texturedata.Texture{nil, owned, other} {
		if err := app.PrepareTexture(image); !errors.Is(err, errTextureOwner) {
			t.Fatalf("PrepareTexture accepted invalid ownership: %v", err)
		}
	}

	if len(driver.images) != 0 {
		t.Fatal("invalid handles reached the backend")
	}
}

func TestPrepareTextureBeforeRunAndOptionalBackend(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	image := batchTexture(t, app)
	if err := app.PrepareTexture(image); err != nil {
		t.Fatal(err)
	}

	app.driver = &struct{ platform.Driver }{}
	defer func() { app.driver = nil }()

	if err := app.PrepareTexture(image); err != nil {
		t.Fatal(err)
	}

	if !texturedata.Valid(image) || len(app.textures) != 1 {
		t.Fatal("preparation changed texture ownership")
	}
}

func TestPrepareTextureDelegatesRepeatedHandleAndErrors(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	image := batchTexture(t, app)
	driver := &texturePrepareDriver{}

	app.driver = driver
	defer func() { app.driver = nil }()

	for range 2 {
		if err := app.PrepareTexture(image); err != nil {
			t.Fatal(err)
		}
	}

	if len(driver.images) != 2 || driver.images[0] != image || driver.images[1] != image {
		t.Fatal("preparation did not delegate the original cached handle")
	}

	driver.err = errTexturePrepareFixture
	if err := app.PrepareTexture(image); !errors.Is(err, driver.err) {
		t.Fatalf("backend error was lost: %v", err)
	}
}

func TestPrepareTextureRejectsClosedApp(t *testing.T) {
	t.Parallel()

	var absent *App
	if err := absent.PrepareTexture(nil); !errors.Is(err, errAppClosed) {
		t.Fatalf("nil app preparation: %v", err)
	}

	app := NewApp(AppConfig{})
	image := batchTexture(t, app)

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if err := app.PrepareTexture(image); !errors.Is(err, errAppClosed) {
		t.Fatalf("closed app preparation: %v", err)
	}
}
