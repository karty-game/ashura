package ui_test

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func scrollSurface(t *testing.T) (*ui.View, *ui.Node) {
	t.Helper()

	panel := ui.Column(scrollItems(30)...).WithStyle(ui.Style{Scroll: true, Height: 180})

	return surface(t, ui.Column(panel)), panel
}

func scrollItems(count int) []*ui.Node {
	items := make([]*ui.Node, count)
	for index := range items {
		items[index] = ui.NewButton("Item", nil)
	}

	return items
}

func TestWheelEasesAndStopsWithoutMeasuring(t *testing.T) {
	t.Parallel()
	view, panel := scrollSurface(t)
	before := view.Stats()

	result := view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 120})
	if !result.Consumed || panel.ScrollOffset() != 0 {
		t.Fatal("wheel should schedule, not jump")
	}

	if _, err := view.Frame(time.Second + 35*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	first := panel.ScrollOffset()
	if first <= 0 || first >= 120 {
		t.Fatal("missing intermediate eased position", first)
	}

	if _, err := view.Frame(time.Second + 70*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	if panel.ScrollOffset() <= first || panel.ScrollOffset() >= 120 {
		t.Fatal("nonmonotonic scroll")
	}

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if panel.ScrollOffset() != 120 || view.Stats().Measures != before.Measures || view.Stats().Shapes != before.Shapes {
		t.Fatal("scroll did not settle or remeasured text", view.Stats(), before)
	}

	stats := view.Stats()
	paint(t, view)

	if view.Stats() != stats {
		t.Fatal("settled scroll did not go idle")
	}
}

func TestWheelReducedMotionAndReverse(t *testing.T) {
	t.Parallel()
	view, panel := scrollSurface(t)
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 200})

	if _, err := view.Frame(time.Second + 35*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	previous := panel.ScrollOffset()

	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: -40})

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if panel.ScrollOffset() != previous-40 {
		t.Fatal("opposite input retained queued motion", panel.ScrollOffset(), previous)
	}

	view.SetReducedMotion(true)
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 60})

	if panel.ScrollOffset() != previous+20 {
		t.Fatal("reduced motion eased instead of applying directly")
	}
}

func TestScrollbarDragAndTouchTakeover(t *testing.T) {
	t.Parallel()
	view, panel := scrollSurface(t)
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 120})

	down := view.Dispatch(ui.Event{Kind: ui.PointerDown, Device: "touch", Pointer: 5, X: 100, Y: 80})
	if !down.Capture {
		t.Fatal("touch did not capture")
	}

	view.Dispatch(ui.Event{Kind: ui.PointerMove, Device: "touch", Pointer: 5, X: 100, Y: 40})
	view.Dispatch(ui.Event{Kind: ui.PointerUp, Device: "touch", Pointer: 5, X: 100, Y: 40})

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if panel.ScrollOffset() != 40 {
		t.Fatal("wheel motion fought direct touch", panel.ScrollOffset())
	}

	bounds := panel.Bounds()
	positionX, positionY := bounds.X+bounds.W-6, bounds.Y+40
	down = view.Dispatch(ui.Event{Kind: ui.PointerDown, Device: "mouse", Pointer: 1, X: positionX, Y: positionY})
	pointer(view, ui.PointerMove, positionX, positionY+50)
	pointer(view, ui.PointerUp, positionX, positionY+50)

	if !down.Capture || panel.ScrollOffset() <= 40 || view.CapturesPointer() {
		t.Fatal("scrollbar drag failed", panel.ScrollOffset())
	}
}

func scrollThumb(t *testing.T, view *ui.View, panel *ui.Node, now time.Duration) (ui.Rect, bool) {
	t.Helper()

	frame, err := view.Frame(now)
	if err != nil {
		t.Fatal(err)
	}

	for _, batch := range frame.Batches {
		for index := range batch.Count {
			offset := (batch.First + index) * uipaint.Stride
			data := frame.Data[offset:]

			var values [4]float32
			for axis := range values {
				values[axis] = math.Float32frombits(binary.LittleEndian.Uint32(data[axis*4:]))
			}

			if values[0] != panel.Bounds().X+panel.Bounds().W-9 || values[2] != 3 {
				continue
			}

			changed := false
			for _, change := range frame.Changes {
				changed = changed || (offset >= change.Offset && offset < change.Offset+change.Size)
			}

			return ui.Rect{X: values[0], Y: values[1], W: values[2], H: values[3]}, changed
		}
	}

	t.Fatal("missing submitted scrollbar thumb")

	return ui.Rect{}, false
}

func TestScrollbarPaintTracksOffsetDuringDragAndWheel(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"drag", "wheel", "reduced-motion wheel"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			view, panel := scrollSurface(t)
			before, _ := scrollThumb(t, view, panel, time.Second)
			stats := view.Stats()

			if scenario == "drag" {
				x, y := before.X+before.W/2, before.Y+before.H/2
				pointer(view, ui.PointerDown, x, y)
				pointer(view, ui.PointerMove, x, y+45)
			} else {
				view.SetReducedMotion(scenario == "reduced-motion wheel")
				view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 120})
			}

			after, changed := scrollThumb(t, view, panel, time.Second+70*time.Millisecond)

			child := panel.Children()[0]
			if !changed || after.Y <= before.Y || child.Bounds().Y != panel.Bounds().Y-panel.ScrollOffset() {
				t.Fatal("submitted thumb did not follow content", before, after, panel.ScrollOffset(), changed)
			}

			if view.Stats().Measures != stats.Measures || view.Stats().Shapes != stats.Shapes {
				t.Fatal("scroll repaint remeasured or reshaped content", stats, view.Stats())
			}
		})
	}
}

func TestWheelBurstStaysInsideOverflowingPanel(t *testing.T) {
	t.Parallel()

	for _, reduced := range []bool{false, true} {
		t.Run(map[bool]string{false: "animated", true: "reduced motion"}[reduced], func(t *testing.T) {
			t.Parallel()

			inner := ui.Column(scrollItems(20)...).WithStyle(ui.Style{Height: 180, Scroll: true})
			outer := ui.Column(inner, ui.Column().WithStyle(ui.Style{Height: 700})).WithStyle(ui.Style{Scroll: true})
			view := surface(t, outer)
			view.SetReducedMotion(reduced)

			for range 12 {
				consumePanelWheel(t, view, 300)
			}

			if _, err := view.Frame(2 * time.Second); err != nil {
				t.Fatal(err)
			}

			if inner.ScrollOffset() <= 0 || outer.ScrollOffset() != 0 {
				t.Fatal("wheel burst scrolled outer page", inner.ScrollOffset(), outer.ScrollOffset())
			}

			for index, delta := range []float32{300, -10000, -300} {
				consumePanelWheel(t, view, delta)

				if _, err := view.Frame(time.Duration(3+index) * time.Second); err != nil {
					t.Fatal(err)
				}
			}

			if inner.ScrollOffset() != 0 || outer.ScrollOffset() != 0 {
				t.Fatal("boundary input did not stay inside the panel", inner.ScrollOffset(), outer.ScrollOffset())
			}

			view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 250, DeltaY: 100})

			if _, err := view.Frame(6 * time.Second); err != nil {
				t.Fatal(err)
			}

			if outer.ScrollOffset() != 100 {
				t.Fatal("wheel outside nested panel did not scroll the page", outer.ScrollOffset())
			}
		})
	}
}

func consumePanelWheel(t *testing.T, view *ui.View, delta float32) {
	t.Helper()

	if !view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: delta}).Consumed {
		t.Fatal("overflowing panel released wheel input to the page", delta)
	}
}

func TestWheelPassesThroughPanelWithoutOverflow(t *testing.T) {
	t.Parallel()

	inner := ui.Column(ui.NewButton("Short content", nil)).WithStyle(ui.Style{Height: 180, Scroll: true})
	outer := ui.Column(inner, ui.Column().WithStyle(ui.Style{Height: 700})).WithStyle(ui.Style{Scroll: true})
	view := surface(t, outer)
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: 100, Y: 80, DeltaY: 100})

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if inner.ScrollOffset() != 0 || outer.ScrollOffset() != 100 {
		t.Fatal("non-overflowing panel trapped wheel input", inner.ScrollOffset(), outer.ScrollOffset())
	}
}
