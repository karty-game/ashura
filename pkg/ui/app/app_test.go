package uiapp

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

func TestAttachPreservesViewAndHonorsAppLifetime(t *testing.T) {
	t.Parallel()

	face, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	view, err := ui.New(ui.Column(), face)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	if View(app) != nil || View(nil) != nil {
		t.Fatal("unattached application has a retained View")
	}

	if err := Attach(app, view); err != nil || View(app) != view {
		t.Fatal("attachment did not preserve the caller's View", err)
	}

	if err := Attach(app, nil); err != nil || View(app) != nil || app.UI() != nil {
		t.Fatal("nil View did not detach UI", err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if err := Attach(app, view); !errors.Is(err, engine.ErrClosed) {
		t.Fatal("closed application accepted UI", err)
	}

	if err := Attach(nil, view); !errors.Is(err, engine.ErrClosed) {
		t.Fatal("nil application accepted UI", err)
	}
}
