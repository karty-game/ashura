package ui_test

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

type motionSubmittedRecord struct {
	rect, clip, uv      ui.Rect
	color, border       ui.Color
	radius, width, mode float32
}

func motionRecords(t *testing.T, view *ui.View, now time.Duration) ([]motionSubmittedRecord, uint64) {
	t.Helper()

	frame, err := view.Frame(now)
	if err != nil {
		t.Fatal(err)
	}

	var records []motionSubmittedRecord

	for _, batch := range frame.Batches {
		for index := range batch.Count {
			data := frame.Data[(batch.First+index)*uipaint.Stride:]
			value := func(offset int) float32 {
				return math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
			}
			rect := func(offset int) ui.Rect {
				return ui.Rect{X: value(offset), Y: value(offset + 4), W: value(offset + 8), H: value(offset + 12)}
			}
			color := func(offset int) ui.Color {
				return ui.Color{value(offset), value(offset + 4), value(offset + 8), value(offset + 12)}
			}
			records = append(records, motionSubmittedRecord{
				rect: rect(0), clip: rect(16), color: color(32), border: color(48), uv: rect(64),
				radius: value(80), width: value(84), mode: value(88),
			})
		}
	}

	return records, frame.Revision
}

func motionSurfaceRecord(t *testing.T, records []motionSubmittedRecord, node *ui.Node) motionSubmittedRecord {
	t.Helper()

	for _, record := range records {
		if record.mode == 0 && record.rect == node.Bounds() {
			return record
		}
	}

	t.Fatal("missing submitted control surface")

	return motionSubmittedRecord{}
}

func motionRipple(records []motionSubmittedRecord) (motionSubmittedRecord, bool) {
	for _, record := range records {
		if record.mode == 6 {
			return record, true
		}
	}

	return motionSubmittedRecord{}, false
}

func TestMotionReviewHoverRetargetIsContinuousAndSettles(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Animate", nil)
	view := surface(t, ui.Column(button))
	before := view.Stats()
	initial, _ := motionRecords(t, view, time.Second)
	idle := motionSurfaceRecord(t, initial, button).color
	positionX, positionY := point(button)
	pointer(view, ui.PointerMove, positionX, positionY)
	motionRecords(t, view, time.Second)
	middle, _ := motionRecords(t, view, time.Second+60*time.Millisecond)

	intermediate := motionSurfaceRecord(t, middle, button).color
	if intermediate == idle || intermediate == view.Theme().SecondaryButton.Hover {
		t.Fatal("hover skipped intermediate displayed colors", idle, intermediate)
	}

	pointer(view, ui.PointerMove, 510, 390)

	retarget, _ := motionRecords(t, view, time.Second+60*time.Millisecond)
	if motionSurfaceRecord(t, retarget, button).color != intermediate {
		t.Fatal("retarget jumped away from the displayed color")
	}

	settled, revision := motionRecords(t, view, 2*time.Second)
	if motionSurfaceRecord(t, settled, button).color != idle {
		t.Fatal("hover did not return to its resting surface")
	}

	_, later := motionRecords(t, view, 3*time.Second)
	if later != revision || view.Stats().Measures != before.Measures || view.Stats().Shapes != before.Shapes {
		t.Fatal("settled motion performed layout, shaping or another repaint", revision, later, before, view.Stats())
	}
}

func TestMotionReviewRippleKeepsActivationSynchronous(t *testing.T) {
	t.Parallel()

	activations := 0
	button := ui.NewButton("Activate", func() { activations++ })
	view := surface(t, ui.Column(button))
	bounds := button.Bounds()
	positionX, positionY := bounds.X+bounds.W/4, bounds.Y+bounds.H/3
	pointer(view, ui.PointerDown, positionX, positionY)
	motionRecords(t, view, time.Second)
	pressed, _ := motionRecords(t, view, time.Second+70*time.Millisecond)

	wave, found := motionRipple(pressed)
	if !found || wave.uv.X != positionX || wave.uv.Y != positionY || wave.width <= 0 || activations != 0 {
		t.Fatal("pointer wave lost its origin or changed activation timing", wave, found, activations)
	}

	pointer(view, ui.PointerUp, positionX, positionY)

	if activations != 1 {
		t.Fatal("release activation waited for the animation", activations)
	}

	motionRecords(t, view, 2*time.Second)
	key(view, "Enter")

	if activations != 2 {
		t.Fatal("keyboard activation waited for the animation", activations)
	}

	keyboard, _ := motionRecords(t, view, 2*time.Second+40*time.Millisecond)

	wave, found = motionRipple(keyboard)
	if !found || absFloat(wave.uv.X-(bounds.X+bounds.W/2)) > 0.001 || absFloat(wave.uv.Y-(bounds.Y+bounds.H/2)) > 0.001 {
		t.Fatal("keyboard pulse was not centered", wave, found)
	}
}

func TestMotionReviewReducedMotionAndDisableStopWaves(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"reduced motion", "disabled control", "disabled effects"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			button := ui.NewButton("Stop effects", nil)
			view := surface(t, ui.Column(button))
			positionX, positionY := point(button)
			pointer(view, ui.PointerDown, positionX, positionY)
			motionRecords(t, view, time.Second)
			motionRecords(t, view, time.Second+50*time.Millisecond)

			switch mode {
			case "reduced motion":
				view.SetReducedMotion(true)
			case "disabled control":
				button.SetDisabled(true)
			case "disabled effects":
				theme := view.Theme()

				theme.Motion.Disabled = true
				if err := view.SetTheme(theme); err != nil {
					t.Fatal(err)
				}
			}

			records, revision := motionRecords(t, view, 2*time.Second)
			if _, found := motionRipple(records); found {
				t.Fatal("wave survived disabled/reduced state", mode)
			}

			_, later := motionRecords(t, view, 3*time.Second)
			if later != revision {
				t.Fatal("disabled/reduced motion kept repainting", mode, revision, later)
			}
		})
	}
}

func TestMotionReviewInputFocusPreservesEditingGeometry(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Name", "Explorer", "", nil)
	view := surface(t, ui.Column(input))
	before := view.Stats()
	bounds := input.Bounds()
	click(view, input)
	motionRecords(t, view, time.Second)
	initial := view.Editing()
	middle, _ := motionRecords(t, view, time.Second+80*time.Millisecond)
	finished, _ := motionRecords(t, view, time.Second+300*time.Millisecond)

	final := view.Editing()
	if input.Bounds() != bounds || initial.Bounds != final.Bounds || initial.Caret != final.Caret ||
		view.Stats().Measures != before.Measures || view.Stats().Shapes != before.Shapes {
		t.Fatal("focus decoration moved the text/caret or remeasured the form", bounds, input.Bounds(), initial, final)
	}

	indicatorWidth := func(records []motionSubmittedRecord) float32 {
		width := float32(0)

		for _, record := range records {
			if record.mode == 0 && record.rect.H <= 3 && record.rect.W > 2 && record.rect.Y >= bounds.Y+bounds.H-4 {
				width = max(width, record.rect.W)
			}
		}

		return width
	}
	if indicatorWidth(middle) <= 0 || indicatorWidth(middle) >= indicatorWidth(finished) {
		t.Fatal("focus indicator did not grow through intermediate widths", indicatorWidth(middle), indicatorWidth(finished))
	}
}

func absFloat(value float32) float32 {
	if value < 0 {
		return -value
	}

	return value
}
