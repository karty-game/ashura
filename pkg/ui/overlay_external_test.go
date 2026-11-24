package ui_test

import (
	"errors"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestOverlayFitsScrollableContentWithoutChangingRootFlow(t *testing.T) {
	t.Parallel()

	underlying, overlayClicks := 0, 0
	base := ui.NewButton("Underlying", func() { underlying++ }).WithStyle(ui.Style{Height: 120})
	view := surface(t, ui.Column(base))
	baseBounds := base.Bounds()
	header := ui.NewButton("Header", func() { overlayClicks++ }).WithStyle(ui.Style{Height: 44})

	rows := make([]*ui.Node, 30)
	for index := range rows {
		rows[index] = ui.NewText("Retained metric")
	}

	body := ui.Column(rows...).WithStyle(ui.Style{Grow: 1, Scroll: true})
	if err := body.SetFocusable(true); err != nil {
		t.Fatal(err)
	}

	root := ui.Column(header, body).WithStyle(ui.Style{Padding: 8, Gap: 4})

	remove, err := view.AddOverlay(root, ui.OverlayOptions{
		Anchor: ui.OverlayTopRight, Width: 200, MaxHeightRatio: .5, Margin: 10, Cursor: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer remove()

	paint(t, view)

	if root.Bounds() != (ui.Rect{X: 290, Y: 10, W: 200, H: 190}) || base.Bounds() != baseBounds {
		t.Fatal("overlay changed root flow or ignored viewport bounds", root.Bounds(), base.Bounds())
	}

	if body.Bounds().Y+body.Bounds().H > root.Bounds().Y+root.Bounds().H || header.Bounds().H != 44 {
		t.Fatal("capped overlay did not fit its flexible scroll body", header.Bounds(), body.Bounds())
	}

	if !view.ConsumesPointer(291, 11) || view.Dispatch(ui.Event{Kind: ui.Wheel, X: 291, Y: 11, DeltaY: 1}).Consumed != true {
		t.Fatal("overlay chrome passed pointer/wheel input to the world")
	}

	click(view, header)

	if overlayClicks != 1 || underlying != 0 || view.CapturesPointer() || !view.PreventsPointerLock() {
		t.Fatal("overlay input was modal or reached underlying controls")
	}

	headerBounds := header.Bounds()

	view.SetFocus(body)
	key(view, "PageDown")
	paint(t, view)

	if body.ScrollOffset() <= 0 || header.Bounds() != headerBounds {
		t.Fatal("keyboard scroll moved the pinned header or failed to scroll metrics")
	}

	root.SetHidden(true)
	paint(t, view)

	if view.PreventsPointerLock() || view.Focused() != nil {
		t.Fatal("hidden overlay retained cursor/focus ownership")
	}

	if err := view.Resize(150, 100); err != nil {
		t.Fatal(err)
	}

	root.SetHidden(false)
	paint(t, view)

	if root.Bounds().X < 0 || root.Bounds().Y < 0 || root.Bounds().W > 130 || root.Bounds().H > 40 {
		t.Fatal("overlay overflowed a small viewport", root.Bounds())
	}
}

func TestOverlayRemovalClearsCaptureFocusAndStaleIdentity(t *testing.T) {
	t.Parallel()

	opener := ui.NewButton("Existing control", nil)
	view := surface(t, ui.Column(opener))
	before := view.Stats().Nodes
	inside := ui.NewButton("Inside overlay", nil)
	canceled := 0

	inside.OnCancel(func() { canceled++ })
	root := ui.Column(inside)

	remove, err := view.AddOverlay(root, ui.OverlayOptions{Width: 200})
	if err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	positionX, positionY := point(inside)
	pointer(view, ui.PointerDown, positionX, positionY)

	if !view.CapturesPointer() || view.Focused() != inside {
		t.Fatal("overlay did not receive capture/focus")
	}

	identifier := inside.ID()

	root.Remove()
	remove()
	remove()

	if canceled != 1 || view.CapturesPointer() || view.Focused() != nil || view.Lookup(identifier) != nil || view.Stats().Nodes != before {
		t.Fatal("removed overlay retained interaction or identities", canceled, view.Stats())
	}

	for _, semantic := range view.Semantics() {
		if semantic.ID == identifier {
			t.Fatal("removed overlay remained in semantics before the next frame")
		}
	}

	if err := view.Root().Add(root); err != nil {
		t.Fatal("removed overlay could not be remounted as ordinary content", err)
	}

	paint(t, view)

	if inside.ID() == identifier {
		t.Fatal("remounted overlay reused a stale identity")
	}
}

func TestOverlayStaysBelowModalScopeAndRestoresFocus(t *testing.T) {
	t.Parallel()

	opener := ui.NewButton("Opener", nil)
	modalButton := ui.NewButton("Modal", nil)
	dialog := ui.Column(modalButton)
	dialog.SetHidden(true)
	view := surface(t, ui.Column(opener, dialog))
	overlayButton := ui.NewButton("Overlay", nil)

	remove, err := view.AddOverlay(ui.Column(overlayButton), ui.OverlayOptions{Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	defer remove()

	view.SetFocus(opener)

	if err := view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	view.SetFocus(overlayButton)

	if view.Focused() != modalButton {
		t.Fatal("nonmodal overlay escaped the active modal focus scope")
	}

	view.PopModal()

	if view.Focused() != opener {
		t.Fatal("closing modal did not restore existing root focus")
	}
}

func TestGlobalPreviewWithoutFocusAndCancellationDuringDispatch(t *testing.T) {
	t.Parallel()

	view := surface(t, ui.Column())

	var calls []string

	var stopFirst, stopSecond func()

	stopFirst = view.OnPreview(func(event *ui.RoutedEvent) {
		calls = append(calls, "first")

		if event.Target != nil || event.Current != nil {
			t.Fatal("global shortcut required a target or pretended to be a node")
		}

		stopFirst()
		stopSecond()
		view.OnPreview(func(*ui.RoutedEvent) { calls = append(calls, "next") })
		event.PreventDefault()
		event.StopPropagation()
	})
	stopSecond = view.OnPreview(func(*ui.RoutedEvent) { t.Fatal("canceled preview ran") })

	result := view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "F3"})
	if !result.Consumed || result.Err != nil || len(calls) != 1 || calls[0] != "first" {
		t.Fatal("global preview did not consume only the current shortcut", result, calls)
	}

	result = view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "F3"})
	if result.Consumed || len(calls) != 2 || calls[1] != "next" {
		t.Fatal("preview registration/cancellation did not apply at event boundaries", result, calls)
	}
}

func TestOverlayRejectsInvalidOptionsAndFocusKinds(t *testing.T) {
	t.Parallel()

	view := surface(t, ui.Column())
	for _, options := range []ui.OverlayOptions{{Width: -1}, {MaxHeightRatio: 2}, {Margin: -1}, {Anchor: ui.OverlayAnchor(255)}} {
		root := ui.Column(ui.NewText("Detached"))
		if _, err := view.AddOverlay(root, options); !errors.Is(err, ui.ErrBounds) || view.Stats().Nodes != 1 {
			t.Fatal("invalid overlay mutated the view", options, err)
		}

		if err := view.Root().Add(root); err != nil {
			t.Fatal("rejected overlay retained ownership", err)
		}

		root.Remove()
	}

	if err := ui.NewButton("Control", nil).SetFocusable(false); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("control focusability override was accepted", err)
	}
}

//nolint:paralleltest // AllocsPerRun must remain isolated on Go 1.27.
func TestUnchangedOverlayFramesDoNotAllocateOrRelayout(t *testing.T) {
	view := surface(t, ui.Column(ui.NewText("Existing content")))

	remove, err := view.AddOverlay(ui.Column(ui.NewText("Static metric")), ui.OverlayOptions{Width: 200, Cursor: true})
	if err != nil {
		t.Fatal(err)
	}
	defer remove()

	paint(t, view)
	before := view.Stats()

	var frameErr error

	allocations := testing.AllocsPerRun(100, func() {
		_, frameErr = view.Frame(time.Second)
		if !view.PreventsPointerLock() {
			t.Fatal("overlay cursor state changed")
		}

		// Exercise reverse hit/scroll/overlay scans even when the point misses.
		if !view.ConsumesPointer(499, 1) || view.ConsumesPointer(-1, -1) {
			t.Fatal("unchanged overlay changed pointer consumption")
		}
	})
	if allocations != 0 || frameErr != nil || view.Stats() != before {
		t.Fatal("unchanged overlay performed frame work", allocations, frameErr, before, view.Stats())
	}
}
