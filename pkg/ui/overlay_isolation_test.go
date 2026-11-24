package ui_test

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestOverlayChromeBlocksUnderlyingControlsAndScroll(t *testing.T) {
	t.Parallel()

	activated := 0
	button := ui.NewButton("Underlying control", func() { activated++ }).WithStyle(ui.Style{Height: 100})
	content := make([]*ui.Node, 1, 21)

	content[0] = button
	for range 20 {
		content = append(content, ui.NewText("Underlying scrolling content"))
	}

	scroller := ui.Column(content...).WithStyle(ui.Style{Height: 200, Scroll: true})
	view := surface(t, ui.Column(scroller))
	cover := ui.Column(ui.NewText("Noninteractive overlay chrome")).WithStyle(ui.Style{Height: 100})

	remove, err := view.AddOverlay(cover, ui.OverlayOptions{Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	defer remove()

	paint(t, view)

	for _, event := range []ui.Event{
		{Kind: ui.PointerDown, Pointer: 1, X: 310, Y: 10},
		{Kind: ui.PointerUp, Pointer: 1, X: 310, Y: 10},
		{Kind: ui.Wheel, X: 310, Y: 10, DeltaY: 80},
	} {
		result := view.Dispatch(event)
		if !result.Consumed || result.Capture || result.Err != nil {
			t.Fatal("overlay chrome did not isolate its pointer event", result)
		}
	}

	paint(t, view)

	if activated != 0 || scroller.ScrollOffset() != 0 {
		t.Fatal("covered root content handled the overlay's pointer/wheel", activated, scroller.ScrollOffset())
	}

	remove()
	pointer(view, ui.PointerDown, 310, 10)
	pointer(view, ui.PointerUp, 310, 10)

	if activated != 1 {
		t.Fatal("removing the cover did not restore underlying control input")
	}
}

func TestTopmostOverlayChromeBlocksEarlierOverlay(t *testing.T) {
	t.Parallel()

	view := surface(t, ui.Column())
	activated := 0
	button := ui.NewButton("Lower overlay", func() { activated++ }).WithStyle(ui.Style{Height: 100})

	removeLower, err := view.AddOverlay(ui.Column(button), ui.OverlayOptions{Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	defer removeLower()

	upper := ui.Column(ui.NewText("Top overlay")).WithStyle(ui.Style{Height: 100})

	removeUpper, err := view.AddOverlay(upper, ui.OverlayOptions{Width: 200})
	if err != nil {
		t.Fatal(err)
	}
	defer removeUpper()

	paint(t, view)
	pointer(view, ui.PointerDown, 310, 10)
	pointer(view, ui.PointerUp, 310, 10)

	if activated != 0 {
		t.Fatal("top overlay chrome activated an earlier overlay")
	}

	upper.SetHidden(true)
	pointer(view, ui.PointerDown, 310, 10)
	pointer(view, ui.PointerUp, 310, 10)

	if activated != 1 {
		t.Fatal("hiding the top overlay did not restore the earlier overlay")
	}
}

func TestPreviewOnlyShortcutsDoNotRouteKeysOrMoveFocus(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Name", "unchanged", "", nil)
	view := surface(t, ui.Column(input, ui.NewButton("Next", nil)))
	view.SetFocus(input)
	paint(t, view)
	before := view.Stats()

	stop := view.OnPreview(func(event *ui.RoutedEvent) {
		if event.Target != nil || event.Current != nil {
			t.Fatal("preview-only dispatch exposed a node target")
		}

		if event.Event.Key == "F3" {
			if nested := view.Dispatch(ui.Event{Kind: ui.KeyDown, Key: "Tab"}); !errors.Is(nested.Err, ui.ErrReentrant) {
				t.Fatal("full Dispatch was allowed inside preview dispatch", nested)
			}

			if nested := view.DispatchPreview(ui.Event{Kind: ui.KeyDown, Key: "F3"}); !errors.Is(nested.Err, ui.ErrReentrant) {
				t.Fatal("preview Dispatch was allowed recursively", nested)
			}

			event.PreventDefault()
		}
	})
	defer stop()

	for _, event := range []ui.Event{
		{Kind: ui.KeyDown, Key: "Tab"},
		{Kind: ui.KeyDown, Key: "Backspace"},
		{Kind: ui.InsertText, Text: "outside input"},
	} {
		if result := view.DispatchPreview(event); result.Consumed || result.Err != nil {
			t.Fatal("ordinary preview-only event was consumed", result)
		}
	}

	if view.Focused() != input || input.Text() != "unchanged" || view.Stats() != before {
		t.Fatal("preview-only dispatch edited, navigated or laid out the UI")
	}

	if result := view.DispatchPreview(ui.Event{Kind: ui.KeyDown, Key: "F3"}); !result.Consumed || result.Err != nil {
		t.Fatal("global shortcut was not consumed in preview-only dispatch", result)
	}
}
