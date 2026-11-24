package ui_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestUnwrappedTextShapesOnceAcrossMeasurePaintAndResize(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("One retained shape", nil)
	view := surface(t, ui.Column(button))

	before := view.Stats().Shapes
	for _, width := range []float32{300, 600, 450} {
		if err := view.Resize(width, 400); err != nil {
			t.Fatal(err)
		}

		paint(t, view)
	}

	if view.Stats().Shapes != before {
		t.Fatal("unwrapped control reshaped because its rectangle width changed")
	}
}

func TestHotTextSurvivesThousandsOfDistinctUpdates(t *testing.T) {
	t.Parallel()

	stable := ui.NewText("Frequently reused heading")
	changing := ui.NewButton("Initial row", nil)
	view := surface(t, ui.Column(stable, changing))
	before := view.Stats().Shapes

	const updates = 2200
	for index := range updates {
		if err := changing.SetText("Row " + strconv.Itoa(index)); err != nil {
			t.Fatal(err)
		}

		paint(t, view)
	}

	if shapes := view.Stats().Shapes - before; shapes != updates {
		t.Fatal("updates reshaped retained labels or duplicated measure/paint shaping", shapes, updates)
	}
}

//nolint:paralleltest // Allocation counters require serial execution.
func TestOrdinaryWheelDispatchAndRetainedScrollDoNotAllocate(t *testing.T) {
	children := make([]*ui.Node, 100)
	for index := range children {
		children[index] = ui.NewButton("Retained row "+strconv.Itoa(index), nil)
	}

	view := surface(t, ui.Column(children...).WithStyle(ui.Style{Scroll: true}))
	view.SetReducedMotion(true)

	iterations := 0

	allocations := testing.AllocsPerRun(100, func() {
		iterations++

		delta := float32(8)
		if iterations%2 == 0 {
			delta = -delta
		}

		result := view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 100, DeltaY: delta})
		if result.Err != nil || !result.Consumed {
			t.Fatal("wheel dispatch failed", result)
		}

		if _, err := view.Frame(time.Second); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatal("unchanged-text scroll allocates", allocations)
	}
}

func TestLongInputSubmitsOnlyGlyphsIntersectingItsViewport(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Long value", strings.Repeat("W", ui.MaxTextBytes), "", nil)
	view := surface(t, ui.Column(input))

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, batch := range frame.Batches {
		count += batch.Count
	}

	if count <= 1 || count > 100 {
		t.Fatal("horizontal clipping submitted invisible glyphs", count)
	}

	view.SetFocus(input)
	paint(t, view)

	if view.Editing().Text != input.Text() || view.Editing().Cursor != ui.MaxTextBytes {
		t.Fatal("paint culling changed the editing document or selection")
	}
}
