package level_test

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/level"
)

func TestInvalidLoadArgumentIsDistinctFromClosedLifetime(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	loader := level.NewLoader(app)

	if _, err := loader.Begin(nil); !errors.Is(err, level.ErrFormat) || errors.Is(err, level.ErrClosed) {
		t.Fatalf("nil chunk on an open loader must be invalid data: %v", err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := loader.Begin(nil); !errors.Is(err, level.ErrClosed) || !errors.Is(err, engine.ErrClosed) {
		t.Fatalf("closed loader must expose the shared closed category: %v", err)
	}
}
