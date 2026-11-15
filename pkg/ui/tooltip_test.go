package ui_test

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func tooltipBox(t *testing.T, view *ui.View, now time.Duration) (ui.Rect, bool) {
	t.Helper()

	frame, err := view.Frame(now)
	if err != nil {
		t.Fatal(err)
	}

	batch := frame.Batches[len(frame.Batches)-1]
	if batch.Count == 0 {
		return ui.Rect{}, false
	}

	data := frame.Data[batch.First*uipaint.Stride:]

	values := [4]float32{}
	for index := range values {
		values[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
	}

	return ui.Rect{X: values[0], Y: values[1], W: values[2], H: values[3]}, true
}

func TestTooltipDelayPlacementAndRetainedIdle(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Help", nil)
	if err := button.SetTooltip(
		"Longer help stays readable, wraps within the viewport and flips above a target near the bottom.",
	); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(ui.Column().WithStyle(ui.Style{Height: 300}), button))
	positionX, positionY := point(button)
	pointer(view, ui.PointerMove, positionX, positionY)

	if _, visible := tooltipBox(t, view, 2*time.Second); visible {
		t.Fatal("tooltip appeared before its delay")
	}

	if _, visible := tooltipBox(t, view, 2149*time.Millisecond); visible {
		t.Fatal("tooltip appeared early")
	}

	bounds, visible := tooltipBox(t, view, 2150*time.Millisecond)
	if !visible || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > 500 || bounds.Y+bounds.H > 400 || bounds.Y >= button.Bounds().Y {
		t.Fatal("tooltip was missing, unclamped or failed to flip", bounds, visible)
	}

	if view.Focused() != nil {
		t.Fatal("hover help moved focus")
	}

	frame, err := view.Frame(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	revision := frame.Revision
	stats := view.Stats()

	frame, err = view.Frame(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if frame.Revision != revision || view.Stats() != stats {
		t.Fatal("idle tooltip rebuilt retained paint")
	}
}

func TestTooltipHoverSurfaceEscapeAndNoFocusTheft(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Help", nil)
	if err := button.SetTooltip("Helpful text without an extra focus stop."); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(button))
	positionX, positionY := point(button)
	pointer(view, ui.PointerMove, positionX, positionY)
	tooltipBox(t, view, time.Second)

	bounds, visible := tooltipBox(t, view, 1500*time.Millisecond)
	if !visible {
		t.Fatal("hover help did not appear")
	}
	// The small bridge between trigger and help must not close help in transit.
	pointer(view, ui.PointerMove, bounds.X+bounds.W/2, button.Bounds().Y+button.Bounds().H+3)

	if _, visible := tooltipBox(t, view, 1600*time.Millisecond); !visible {
		t.Fatal("tooltip closed while crossing the pointer gap")
	}

	pointer(view, ui.PointerMove, bounds.X+bounds.W/2, bounds.Y+bounds.H/2)

	if _, visible := tooltipBox(t, view, 1700*time.Millisecond); !visible {
		t.Fatal("tooltip content was not hoverable")
	}

	key(view, "Escape")

	if _, visible := tooltipBox(t, view, 3*time.Second); visible {
		t.Fatal("escape did not dismiss help")
	}

	if _, visible := tooltipBox(t, view, 5*time.Second); visible {
		t.Fatal("dismissed stationary tooltip reappeared")
	}

	if view.Focused() != nil {
		t.Fatal("tooltip introduced a focus stop")
	}

	pointer(view, ui.PointerMove, 499, 399)
	tooltipBox(t, view, 6*time.Second)
	pointer(view, ui.PointerMove, positionX, positionY)
	tooltipBox(t, view, 7*time.Second)

	if _, visible := tooltipBox(t, view, 7500*time.Millisecond); !visible {
		t.Fatal("fresh hover did not restore help")
	}
}

func TestTooltipKeyboardAndInvalidation(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Help", nil)
	next := ui.NewButton("Next", nil)

	if err := button.SetTooltip("Keyboard help"); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(button, next))
	key(view, "Tab")
	tooltipBox(t, view, time.Second)

	if _, visible := tooltipBox(t, view, 1500*time.Millisecond); !visible || view.Focused() != button {
		t.Fatal("focus help did not preserve the trigger")
	}

	key(view, "Tab")

	if _, visible := tooltipBox(t, view, 2*time.Second); visible || view.Focused() != next {
		t.Fatal("help survived focus leaving its trigger")
	}

	key(view, "Tab")
	tooltipBox(t, view, 3*time.Second)

	if _, visible := tooltipBox(t, view, 3500*time.Millisecond); !visible {
		t.Fatal("keyboard help failed to reappear")
	}

	button.SetDisabled(true)

	if _, visible := tooltipBox(t, view, 4*time.Second); visible {
		t.Fatal("disabled trigger retained help")
	}

	button.SetDisabled(false)
	positionX, positionY := point(button)
	pointer(view, ui.PointerMove, positionX, positionY)
	tooltipBox(t, view, 5*time.Second)

	if _, visible := tooltipBox(t, view, 5500*time.Millisecond); !visible {
		t.Fatal("enabled trigger did not recover")
	}

	button.Remove()

	if _, visible := tooltipBox(t, view, 6*time.Second); visible {
		t.Fatal("removed trigger retained help")
	}
}

func TestTooltipBoundsValidationAndLiveStyle(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Help", nil)
	if err := button.SetTooltip("Good help"); err != nil {
		t.Fatal(err)
	}

	for _, invalid := range []string{strings.Repeat("x", ui.MaxTextBytes+1), string([]byte{0xff})} {
		if err := button.SetTooltip(invalid); !errors.Is(err, ui.ErrBounds) || button.Tooltip() != "Good help" {
			t.Fatal("invalid help changed the previous value", err)
		}
	}

	view := surface(t, ui.Column(button))
	positionX, positionY := point(button)
	pointer(view, ui.PointerMove, positionX, positionY)
	tooltipBox(t, view, time.Second)
	tooltipBox(t, view, 1500*time.Millisecond)

	if err := button.SetPartStyle(ui.TooltipPart, 0, ui.PartStyle{Fill: ui.Hex(0xff0000), Radius: 6}); err != nil {
		t.Fatal(err)
	}

	frame, err := view.Frame(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	batch := frame.Batches[len(frame.Batches)-1]
	data := frame.Data[batch.First*uipaint.Stride:]
	red := math.Float32frombits(binary.LittleEndian.Uint32(data[32:]))

	green := math.Float32frombits(binary.LittleEndian.Uint32(data[36:]))
	if batch.Count == 0 || red != 1 || green != 0 {
		t.Fatal("live tooltip style was not repainted", red, green)
	}

	if err := button.SetTooltip(""); err != nil {
		t.Fatal(err)
	}

	if _, visible := tooltipBox(t, view, 3*time.Second); visible {
		t.Fatal("cleared help remained visible")
	}
}

func TestTooltipDismissalByInputAndModalScope(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"wheel", "blur", "press", "popup", "modal"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			button := ui.NewButton("Help", nil)
			if err := button.SetTooltip("Help must leave room for the active interaction."); err != nil {
				t.Fatal(err)
			}

			choice := ui.NewSelect("Choice", []ui.Option{{Key: "one", Label: "One"}}, "one", nil)
			dialog := ui.Column(ui.NewButton("Close", nil))
			dialog.SetHidden(true)
			view := surface(t, ui.Column(button, choice, dialog))
			positionX, positionY := point(button)
			pointer(view, ui.PointerMove, positionX, positionY)
			tooltipBox(t, view, time.Second)

			if _, visible := tooltipBox(t, view, 1500*time.Millisecond); !visible {
				t.Fatal("help did not appear before interaction")
			}

			switch scenario {
			case "wheel":
				view.Dispatch(ui.Event{Kind: ui.Wheel, X: positionX, Y: positionY, DeltaY: 20})
			case "blur":
				view.Dispatch(ui.Event{Kind: ui.Blur})
			case "press":
				pointer(view, ui.PointerDown, positionX, positionY)
			case "popup":
				click(view, choice)
			case "modal":
				if err := view.PushModal(dialog); err != nil {
					t.Fatal(err)
				}
			}

			if _, visible := tooltipBox(t, view, 2*time.Second); visible {
				t.Fatal("interaction retained stale help")
			}

			if _, visible := tooltipBox(t, view, 4*time.Second); visible {
				t.Fatal("suppressed help returned without a new trigger")
			}
		})
	}
}

func TestTooltipFadeUsesRetainedTextAndSettles(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Help", nil)
	if err := button.SetTooltip("Fast, gentle help."); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(button))
	positionX, positionY := point(button)
	pointer(view, ui.PointerMove, positionX, positionY)
	tooltipBox(t, view, time.Second)
	tooltipBox(t, view, 1150*time.Millisecond)
	stats := view.Stats()

	for _, sample := range []struct {
		time    time.Duration
		opacity float32
	}{
		{1150 * time.Millisecond, 0},
		{1180 * time.Millisecond, 0.578125},
		{1210 * time.Millisecond, 0.875},
		{1270 * time.Millisecond, 1},
	} {
		frame, err := view.Frame(sample.time)
		if err != nil {
			t.Fatal(err)
		}

		batch := frame.Batches[len(frame.Batches)-1]
		if batch.Count < 2 {
			t.Fatal("missing tooltip box and text during fade")
		}

		for index := range batch.Count {
			data := frame.Data[(batch.First+index)*uipaint.Stride:]

			alpha := math.Float32frombits(binary.LittleEndian.Uint32(data[44:]))
			if alpha != sample.opacity {
				t.Fatal("tooltip box/text alpha did not follow fade", sample.time, index, alpha)
			}
		}

		borderAlpha := math.Float32frombits(binary.LittleEndian.Uint32(frame.Data[batch.First*uipaint.Stride+60:]))
		if borderAlpha != sample.opacity {
			t.Fatal("tooltip border did not fade with fill", sample.time, borderAlpha)
		}
	}

	if view.Stats() != stats {
		t.Fatal("tooltip fade remeasured, reshaped or repainted other controls", stats, view.Stats())
	}

	frame, err := view.Frame(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	revision := frame.Revision

	frame, err = view.Frame(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if frame.Revision != revision {
		t.Fatal("settled tooltip continued uploading paint")
	}
}

func TestTooltipReducedMotionSkipsFade(t *testing.T) {
	t.Parallel()

	for _, changeDuringFade := range []bool{false, true} {
		button := ui.NewButton("Help", nil)
		if err := button.SetTooltip("Still help, without animation."); err != nil {
			t.Fatal(err)
		}

		view := surface(t, ui.Column(button))
		view.SetReducedMotion(!changeDuringFade)

		positionX, positionY := point(button)
		pointer(view, ui.PointerMove, positionX, positionY)
		tooltipBox(t, view, time.Second)

		if changeDuringFade {
			tooltipBox(t, view, 1180*time.Millisecond)
			view.SetReducedMotion(true)
		}

		frame, err := view.Frame(1190 * time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}

		batch := frame.Batches[len(frame.Batches)-1]
		if batch.Count == 0 {
			t.Fatal("reduced motion suppressed tooltip")
		}

		alpha := math.Float32frombits(binary.LittleEndian.Uint32(frame.Data[batch.First*uipaint.Stride+44:]))
		if alpha != 1 {
			t.Fatal("reduced motion retained a partial fade", changeDuringFade, alpha)
		}
	}
}
