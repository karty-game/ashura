package main

import (
	"fmt"
	"time"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/app"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
	"github.com/karty-game/ashura/samples/internal/sample"
)

func setupUI(app *engine.App) error {
	art, err := loadArtwork()
	if err != nil {
		return fmt.Errorf("load UI artwork: %w", err)
	}

	font, err := defaultfont.Load()
	if err != nil {
		return fmt.Errorf("load UI font: %w", err)
	}

	var failures sample.Errors

	root, bind := buildInterface(art, failures.Check)

	view, err := ui.New(root, font)
	if err != nil {
		return fmt.Errorf("build UI: %w", err)
	}

	bind(view)

	if err := failures.Err(); err != nil {
		return err
	}

	if err := uiapp.Attach(app, view); err != nil {
		return err
	}

	app.OnUpdateError(func(time.Duration) error { return failures.Err() })

	return nil
}
