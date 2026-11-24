package engine

import (
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

func TestCursorOverlayDisablesPointerLockWithoutCapturingWorldInput(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	view, err := ui.New(ui.Column(), font)
	if err != nil {
		t.Fatal(err)
	}

	root := ui.Column(ui.NewText("Debug"))

	remove, err := view.AddOverlay(root, ui.OverlayOptions{Width: 100, Cursor: true})
	if err != nil {
		t.Fatal(err)
	}
	defer remove()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	attached := &testUIAttachment{View: view}
	if err := app.SetUI(attached); err != nil || app.UI() != attached {
		t.Fatal("App did not expose its attached View", err)
	}

	frame := render.Frame{PointerLock: true}

	input := platform.InputState{MouseX: 20, MouseY: 200, MouseDX: 2}
	if err := app.composeUI(&frame, input, time.Second, 500, 400); err != nil || frame.PointerLock {
		t.Fatal("visible cursor overlay kept pointer lock", err)
	}

	if filtered := app.filterUI(input); filtered.MouseDX != input.MouseDX {
		t.Fatal("nonmodal cursor overlay captured world input outside its bounds")
	}

	root.SetHidden(true)

	frame.PointerLock = true
	if err := app.composeUI(&frame, input, time.Second, 500, 400); err != nil || !frame.PointerLock {
		t.Fatal("hidden overlay prevented world pointer lock", err)
	}
}
