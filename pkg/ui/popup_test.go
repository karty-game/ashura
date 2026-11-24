package ui_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestPopupWheelAndTouchScrollRetainOffset(t *testing.T) {
	t.Parallel()

	options := make([]ui.Option, 20)
	for index := range options {
		options[index] = ui.Option{Key: strconv.Itoa(index), Label: fmt.Sprintf("Option %02d", index)}
	}

	options[0].Disabled = true
	changes := 0
	node := ui.NewSelect("Choice", options, "1", func(ui.Change) { changes++ })
	view := surface(t, ui.Column(node))
	click(view, node)
	paint(t, view)

	positionX := node.Bounds().X + 20
	positionY := node.Bounds().Y + node.Bounds().H + 5 + view.Theme().ControlHeight/2
	view.Dispatch(ui.Event{Kind: ui.Wheel, X: positionX, Y: positionY, DeltaY: view.Theme().ControlHeight * 4})
	paint(t, view)
	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerUp, positionX, positionY)

	if node.Selected() != "4" || changes != 1 {
		t.Fatal("painting reset the popup's manual scroll", node.Selected(), changes)
	}

	if err := node.SetSelected("1"); err != nil {
		t.Fatal(err)
	}

	click(view, node)
	paint(t, view)
	// Start on a disabled option: it must still allow touch scrolling.
	down := view.Dispatch(ui.Event{Kind: ui.PointerDown, Pointer: 8, Device: "touch", X: positionX, Y: positionY})
	if !down.Capture {
		t.Fatal("disabled row did not allow a scroll gesture")
	}

	view.Dispatch(ui.Event{Kind: ui.PointerMove, Pointer: 8, Device: "touch", X: positionX, Y: positionY - view.Theme().ControlHeight*3})
	view.Dispatch(ui.Event{Kind: ui.PointerUp, Pointer: 8, Device: "touch", X: positionX, Y: positionY - view.Theme().ControlHeight*3})
	paint(t, view)

	if node.Selected() != "1" || changes != 1 || view.CapturesPointer() {
		t.Fatal("scroll gesture accepted an option or leaked capture")
	}

	pointer(view, ui.PointerDown, positionX, positionY)
	pointer(view, ui.PointerUp, positionX, positionY)

	if node.Selected() != "3" || changes != 2 {
		t.Fatal("touch scroll offset was lost", node.Selected(), changes)
	}
}
