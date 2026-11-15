package ui_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

var errVirtualReviewBinding = errors.New("temporary model error")

func TestVirtualReviewViewportGrowthPreservesVisibleItemFocus(t *testing.T) {
	t.Parallel()

	view, list, _ := virtualSurface(t, 10000)
	row := list.Children()[2]
	oldID, text := row.ID(), row.Text()
	view.SetFocus(row)

	if err := view.Resize(500, 480); err != nil {
		t.Fatal(err)
	}

	if _, err := view.Frame(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	if view.Focused() != row || row.ID() != oldID || row.Text() != text {
		t.Fatal("a still-visible logical item lost focus/identity when its viewport grew", row.ID(), oldID, row.Text(), text)
	}
}

func TestVirtualReviewTouchScrollContinuesAcrossRecycledRows(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	activations := 0
	list := ui.NewVirtualList(ui.VirtualListOptions{
		Count: 10000, RowHeight: 40,
		Create: func() *ui.Node { return ui.NewButton("", func() { activations++ }) },
		Bind:   func(row *ui.Node, index int) error { return row.SetText("Result " + strconv.Itoa(index)) },
	})
	view := surfaceWithFont(t, list, font)
	view.SetReducedMotion(true)

	down := view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 42, Device: "touch", X: 100, Y: 140})
	if !down.Capture {
		t.Fatal("touch did not capture a row", down)
	}

	first := view.Dispatch(ui.Event{Kind: ui.PointerMove, Pointer: 42, Device: "touch", X: 100, Y: -100})
	if first.Err != nil {
		t.Fatal(first.Err)
	}

	paint(t, view)

	before := list.ScrollOffset()
	if before <= 0 || !view.CapturesPointer() {
		t.Fatal("recycling lost scrolling capture", before, view.CapturesPointer())
	}

	second := view.Dispatch(ui.Event{Kind: ui.PointerMove, Pointer: 42, Device: "touch", X: 100, Y: -180})
	paint(t, view)

	if second.Err != nil || list.ScrollOffset() != before+80 {
		t.Fatal("continued drag failed after recycling", second, list.ScrollOffset(), before)
	}

	view.Dispatch(ui.Event{Kind: ui.PointerUp, Pointer: 42, Device: "touch", X: 100, Y: -180})

	if view.CapturesPointer() || activations != 0 {
		t.Fatal("touch scroll activated a rebound item or retained capture", activations)
	}
}

func TestVirtualReviewEmptyCollectionCannotFocusRetiredItem(t *testing.T) {
	t.Parallel()

	view, list, _ := virtualSurface(t, 10000)

	row := list.Children()[0]
	if err := list.SetVirtualCount(0); err != nil {
		t.Fatal(err)
	}

	paint(t, view)
	view.SetFocus(row)

	if view.Focused() == row {
		t.Fatal("an inactive pool row remains eligible after the logical collection becomes empty")
	}
}

func TestVirtualReviewBindingFailureCanRecoverAtAnotherWindow(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	failed := false
	list := ui.NewVirtualList(ui.VirtualListOptions{
		Count: 10000, RowHeight: 40,
		Create: func() *ui.Node { return ui.NewButton("", nil) },
		Bind: func(row *ui.Node, index int) error {
			if !failed && index >= 100 {
				failed = true

				return errVirtualReviewBinding
			}

			return row.SetText("Result " + strconv.Itoa(index))
		},
	})

	view := surfaceWithFont(t, list, font)
	if err = list.ScrollToRow(100); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(2 * time.Second); !errors.Is(err, errVirtualReviewBinding) {
		t.Fatal("binding failure was not propagated", err)
	}

	if err = list.ScrollToRow(500); err != nil {
		t.Fatal(err)
	}

	if _, err = view.Frame(3 * time.Second); err != nil {
		t.Fatal("failed binding corrupted the recycled pool", err)
	}

	if list.VirtualListStats().First != 498 || list.VirtualListStats().End != 512 {
		t.Fatal("recovery did not reach the new window", list.VirtualListStats())
	}
}

func TestVirtualReviewCancelObservesRetiredLogicalIdentity(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	indices := make(map[*ui.Node]*int)
	observed := -1
	list := ui.NewVirtualList(ui.VirtualListOptions{
		Count: 10000, RowHeight: 40,
		Create: func() *ui.Node {
			index := -1
			row := ui.NewButton("", nil)
			row.OnCancel(func() { observed = index })
			indices[row] = &index

			return row
		},
		Bind: func(row *ui.Node, index int) error {
			*indices[row] = index

			return row.SetText("Result " + strconv.Itoa(index))
		},
	})
	view := surfaceWithFont(t, list, font)

	result := view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 24, X: 100, Y: 20})
	if !result.Capture {
		t.Fatal("row press did not capture", result)
	}

	if err = list.ScrollToRow(5000); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if observed != 0 {
		t.Fatal("cancel callback observed the replacement logical item", observed)
	}
}

func TestVirtualReviewCompositionCancelObservesRetiredLogicalIdentity(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	indices := make(map[*ui.Node]*int)
	observed := -1
	list := ui.NewVirtualList(ui.VirtualListOptions{
		Count: 10000, RowHeight: 40,
		Create: func() *ui.Node {
			index := -1
			row := ui.NewInput("Result name", "", "", func(change ui.Change) {
				if !change.Composing {
					observed = index
				}
			})
			indices[row] = &index

			return row
		},
		Bind: func(row *ui.Node, index int) error {
			*indices[row] = index

			return row.SetText("Result " + strconv.Itoa(index))
		},
	})
	view := surfaceWithFont(t, list, font)

	for row, index := range indices {
		if *index == 0 {
			view.SetFocus(row)

			break
		}
	}

	view.Dispatch(ui.Event{Kind: ui.CompositionStart})
	view.Dispatch(ui.Event{Kind: ui.InsertText, Text: "pending"})

	if !view.Editing().Composing {
		t.Fatal("input did not begin composition")
	}

	if err = list.ScrollToRow(5000); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if observed != 0 {
		t.Fatal("composition cancellation observed the replacement logical item", observed)
	}
}

func TestVirtualReviewRetiredRowModalCannotMoveToReplacementItem(t *testing.T) {
	t.Parallel()

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	dialogs := make(map[*ui.Node]*ui.Node)
	indices := make(map[*ui.Node]int)
	list := ui.NewVirtualList(ui.VirtualListOptions{
		Count: 10000, RowHeight: 80,
		Create: func() *ui.Node {
			dialog := ui.Column(ui.NewButton("Confirm", nil))
			dialog.SetHidden(true)
			row := ui.Column(ui.NewButton("Open details", nil), dialog)
			dialogs[row] = dialog

			return row
		},
		Bind: func(row *ui.Node, index int) error {
			indices[row] = index

			return nil
		},
	})
	view := surfaceWithFont(t, list, font)

	var dialog *ui.Node

	for row, index := range indices {
		if index == 0 {
			dialog = dialogs[row]

			break
		}
	}

	if dialog == nil {
		t.Fatal("first item was not mounted")
	}

	if err = view.PushModal(dialog); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if !view.CapturesKeyboard() {
		t.Fatal("row dialog was not modal")
	}

	if err = list.ScrollToRow(5000); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if view.CapturesKeyboard() || view.CapturesPointer() {
		t.Fatal("recycled row retained a modal scope for the replacement logical item")
	}

	for _, semantic := range view.Semantics() {
		if semantic.Name == "Confirm" {
			t.Fatal("retired row dialog reappeared on the replacement logical item", semantic)
		}
	}
}

func surfaceWithFont(t *testing.T, root *ui.Node, font *ui.Font) *ui.View {
	t.Helper()

	view, err := ui.New(root, font)
	if err != nil {
		t.Fatal(err)
	}

	if err = view.Resize(500, 400); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	return view
}
