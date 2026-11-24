package ui_test

import (
	"errors"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestRowStyleKeepsDirectionAndRetainsSiblingRecords(t *testing.T) {
	t.Parallel()

	first := ui.NewButton("First", nil)
	second := ui.NewSlider("Value", 0, 10, 1, 5, nil)

	view := surface(t, ui.Row(first, second).WithStyle(ui.Style{Gap: 8}))
	if second.Bounds().X <= first.Bounds().X || second.Bounds().Y != first.Bounds().Y {
		t.Fatal("styling reset the row direction")
	}

	before := view.Stats()

	if err := second.SetValue(6); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	after := view.Stats()
	if after.Layouts != before.Layouts || after.Shapes != before.Shapes || after.Paints != before.Paints+1 {
		t.Fatal("range paint disturbed siblings", before, after)
	}

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if len(frame.Changes) != 1 {
		t.Fatal("range update rewrote unrelated blocks", len(frame.Changes))
	}
}
func TestRemovedRecordStorageIsReusable(t *testing.T) {
	t.Parallel()

	root := ui.Column()
	view := surface(t, root)

	var previous ui.ID

	for range 1000 {
		node := ui.NewButton("Repeated", nil)
		if err := root.Add(node); err != nil {
			t.Fatal(err)
		}

		paint(t, view)

		if node.ID() <= previous {
			t.Fatal("identity reused")
		}

		previous = node.ID()
		node.Remove()
		paint(t, view)
	}

	if view.Stats().Records > 64 {
		t.Fatal("removed slots leaked", view.Stats())
	}
}
func TestLateEditingResultAfterRefocusIsRejected(t *testing.T) {
	t.Parallel()

	input := ui.NewInput("Name", "original", "", nil)
	view := surface(t, ui.Column(input))
	view.SetFocus(input)
	old := view.Editing()
	view.SetFocus(nil)
	view.SetFocus(input)
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "stale", Session: old.Session, Revision: old.Revision})

	if input.Text() != "original" {
		t.Fatal("old focus session modified the editor")
	}
}
func TestModalEscapeAndReentrantDispatch(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Open", nil)
	closeButton := ui.NewButton("Close", nil)
	dialog := ui.Column(closeButton).WithStyle(ui.Style{Width: 300})
	dialog.SetHidden(true)
	view := surface(t, ui.Column(button, dialog))
	view.SetFocus(button)

	if err := view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.Focused() != closeButton || closeButton.Bounds().X <= 0 {
		t.Fatal("modal was not centered/focused")
	}

	key(view, "Escape")
	paint(t, view)

	if view.Focused() != button || view.CapturesKeyboard() {
		t.Fatal("modal failed to restore focus")
	}

	var nested error

	button.OnPreview(func(event *ui.RoutedEvent) {
		nested = view.Dispatch(ui.Event{Kind: ui.Activate, Target: button.ID()}).Err
	})
	click(view, button)

	if !errors.Is(nested, ui.ErrReentrant) {
		t.Fatal("nested dispatch was not explicitly rejected", nested)
	}
}
