package ui_test

import (
	"math"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestCursorControlsAndLifetime(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Run", nil)
	input := ui.NewInput("Name", "", "", nil)
	slider := ui.NewSlider("Volume", 0, 100, 1, 50, nil)

	view := surface(t, ui.Column(button, input, slider, ui.NewText("Plain text")))
	for _, test := range []struct {
		node   *ui.Node
		cursor ui.Cursor
	}{{button, ui.PointerCursor}, {input, ui.TextCursor}, {slider, ui.HorizontalResizeCursor}} {
		positionX, positionY := point(test.node)
		if got := view.CursorAt(positionX, positionY); got != test.cursor {
			t.Fatalf("%v cursor = %q, want %q", test.node.Kind(), got, test.cursor)
		}
	}

	positionX, positionY := point(button)
	button.SetDisabled(true)

	if got := view.CursorAt(positionX, positionY); got != ui.DefaultCursor {
		t.Fatal("disabled control retained interactive cursor", got)
	}

	button.SetDisabled(false)
	button.SetHidden(true)

	if got := view.CursorAt(positionX, positionY); got != ui.DefaultCursor {
		t.Fatal("hidden control retained interactive cursor", got)
	}

	for _, coordinates := range [][2]float32{{-10, -10}, {510, 410}, {float32(math.NaN()), 0}} {
		if got := view.CursorAt(coordinates[0], coordinates[1]); got != ui.DefaultCursor {
			t.Fatal("outside surface retained interactive cursor", got)
		}
	}
}

func TestCursorCaptureAndReadOnly(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Name", "", "", nil)
	slider := ui.NewSlider("Volume", 0, 100, 1, 50, nil)
	view := surface(t, ui.Column(input, slider))
	positionX, positionY := point(slider)
	pointer(view, ui.PointerDown, positionX, positionY)

	if got := view.CursorAt(-10, -10); got != ui.HorizontalResizeCursor {
		t.Fatal("drag lost manipulation cursor outside slider", got)
	}

	pointer(view, ui.PointerUp, -10, -10)

	if got := view.CursorAt(-10, -10); got != ui.DefaultCursor {
		t.Fatal("released drag retained manipulation cursor", got)
	}

	slider.SetReadOnly(true)

	if got := view.CursorAt(positionX, positionY); got != ui.DefaultCursor {
		t.Fatal("read-only slider advertised manipulation", got)
	}

	input.SetReadOnly(true)

	positionX, positionY = point(input)
	if got := view.CursorAt(positionX, positionY); got != ui.TextCursor {
		t.Fatal("read-only text lost selection cursor", got)
	}
}
