package level_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/level"
)

func TestPublicPreparedPresentationContractNeedsNoDeviceToOpenOrClose(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	changes := 0

	presentation, err := level.Open(t.Context(), app, engine.NewWorld(), level.OpenOptions{
		Variants: level.TextureVariants{}, Base: t.TempDir(), Select: level.WholeScene,
		StatusChanged: func(level.PresentationStatus) { changes++ },
	})
	if err != nil {
		t.Fatal(err)
	}

	cancelUpdate := app.OnUpdateError(presentation.Update)
	cancelUpdate()

	if presentation.Status().Closed || presentation.Status().Ready {
		t.Fatal("invalid newly opened presentation status")
	}

	if err := presentation.Close(); err != nil || changes != 0 || presentation.Status() != (level.PresentationStatus{Closed: true}) {
		t.Fatal("Close emitted progress or retained incoherent counts", err, changes, presentation.Status())
	}
}
