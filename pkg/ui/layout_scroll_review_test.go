package ui_test

import (
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func TestWheelReversalBeforeAFrameCancelsQueuedMotion(t *testing.T) {
	t.Parallel()

	view, panel := scrollSurface(t)
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 120})
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: -40})

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if panel.ScrollOffset() != 0 {
		t.Fatal("queued forward motion survived opposite input at the top edge", panel.ScrollOffset())
	}
}

func TestDisabledScrollerRejectsDirectManipulation(t *testing.T) {
	t.Parallel()

	t.Run("reduced-motion wheel", func(t *testing.T) {
		t.Parallel()

		view, panel := scrollSurface(t)
		panel.SetDisabled(true)
		view.SetReducedMotion(true)
		view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 100})

		if panel.ScrollOffset() != 0 {
			t.Fatal("disabled panel accepted wheel input", panel.ScrollOffset())
		}
	})
	t.Run("preference change after disable", func(t *testing.T) {
		t.Parallel()
		view, panel := scrollSurface(t)
		view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 120})
		panel.SetDisabled(true)
		view.SetReducedMotion(true)

		if panel.ScrollOffset() != 0 {
			t.Fatal("reduced-motion preference finished a disabled panel's queued scroll", panel.ScrollOffset())
		}
	})

	t.Run("scrollbar", func(t *testing.T) {
		t.Parallel()

		view, panel := scrollSurface(t)
		panel.SetDisabled(true)
		bounds := panel.Bounds()

		result := view.Dispatch(ui.Event{
			Kind: ui.PointerDown, Pointer: 1, Device: "mouse", X: bounds.X + bounds.W - 6, Y: bounds.Y + 40,
		})
		if result.Capture || view.CapturesPointer() {
			t.Fatal("disabled panel captured a scrollbar drag")
		}
	})
}

func TestPanelScrollCaptureSurvivesUnrelatedStateChange(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"scrollbar", "blank touch"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			view, panel := scrollSurface(t)

			other := ui.NewButton("Other", nil)
			if err := view.Root().Add(other); err != nil {
				t.Fatal(err)
			}

			paint(t, view)

			bounds := panel.Bounds()

			event := ui.Event{Kind: ui.PointerDown, Pointer: 7, Device: "mouse", X: bounds.X + bounds.W - 6, Y: bounds.Y + 40}
			if scenario == "blank touch" {
				event.Device, event.X, event.Y = "touch", 100, 46
			}

			if !view.Dispatch(event).Capture {
				t.Fatal("initial scroll contact did not capture")
			}

			other.SetDisabled(true)

			if !view.CapturesPointer() {
				t.Fatal("unrelated state change canceled a valid panel scroll contact", scenario)
			}

			event.Kind = ui.PointerCancel
			view.Dispatch(event)

			if view.CapturesPointer() {
				t.Fatal("explicit cancellation did not release panel contact")
			}
		})
	}
}

func TestFocusRevealSupersedesPendingWheelMotion(t *testing.T) {
	t.Parallel()

	view, panel := scrollSurface(t)
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 120})

	children := panel.Children()
	last := children[len(children)-1]
	view.SetFocus(last)

	revealed := panel.ScrollOffset()
	if revealed <= 120 {
		t.Fatal("fixture did not reveal its last control", revealed)
	}

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if panel.ScrollOffset() != revealed || last.Bounds().Y+last.Bounds().H > panel.Bounds().Y+panel.Bounds().H {
		t.Fatal("old wheel motion pulled focused control out of view", panel.ScrollOffset(), revealed, last.Bounds())
	}
}

func TestFocusRevealKeepsMeasuredLayout(t *testing.T) {
	t.Parallel()

	view, panel := scrollSurface(t)
	before := view.Stats()
	children := panel.Children()
	view.SetFocus(children[len(children)-1])
	paint(t, view)

	if view.Stats().Measures != before.Measures || view.Stats().Shapes != before.Shapes {
		t.Fatal("revealing focus remeasured unchanged content", before, view.Stats())
	}
}

func TestGrowingRowRedistributesSpaceAfterWidthCap(t *testing.T) {
	t.Parallel()

	limited := ui.NewButton("Limited", nil).WithStyle(ui.Style{Grow: 1, MaxWidth: 100})
	fill := ui.NewButton("Fill", nil).WithStyle(ui.Style{Grow: 1})

	view := surface(t, ui.Row(limited, fill).WithStyle(ui.Style{Gap: 10}))
	if limited.Bounds().W > 100 || fill.Bounds().X+fill.Bounds().W != view.Root().Bounds().W {
		t.Fatal("width cap left distributable space unused", limited.Bounds(), fill.Bounds(), view.Root().Bounds())
	}
}

func TestWrappingRowExcludesHiddenFields(t *testing.T) {
	t.Parallel()

	first := ui.NewField("First", ui.NewInput("", "", "", nil))
	hidden := ui.NewField("Hidden", ui.NewInput("", "", "", nil))
	last := ui.NewField("Last", ui.NewInput("", "", "", nil))
	view := surface(t, ui.NewForm(ui.Row(first, hidden, last).WithStyle(ui.Style{Gap: 16, Wrap: true})))
	hidden.SetHidden(true)
	paint(t, view)

	if first.Bounds().Y != last.Bounds().Y || last.Bounds().X <= first.Bounds().X || last.Bounds().X+last.Bounds().W > 500 {
		t.Fatal("hidden field still occupied a wrapping slot", first.Bounds(), last.Bounds())
	}

	if err := view.Resize(250, 600); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if last.Bounds().Y <= first.Bounds().Y || first.Bounds().W > 250 || last.Bounds().W > 250 {
		t.Fatal("hidden-field form did not resize into one column", first.Bounds(), last.Bounds())
	}
}
