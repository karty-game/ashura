package ui_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestResponsiveRowWrapsByPreferredWidth(t *testing.T) {
	t.Parallel()

	first := ui.Column(ui.NewText("First field"), ui.NewInput("First", "", "", nil)).WithStyle(ui.Style{Basis: 220, Grow: 1})
	second := ui.Column(ui.NewText("Second field"), ui.NewInput("Second", "", "", nil)).WithStyle(ui.Style{Basis: 220, Grow: 1})
	row := ui.Row(first, second).WithStyle(ui.Style{Wrap: true, Gap: 16})

	view := surface(t, ui.Column(row))
	if first.Bounds().Y != second.Bounds().Y || first.Bounds().W != 242 || second.Bounds().X != 258 {
		t.Fatal("wide layout did not share the row", first.Bounds(), second.Bounds())
	}

	if err := view.Resize(320, 600); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if first.Bounds().W != 320 || second.Bounds().X != first.Bounds().X || second.Bounds().Y < first.Bounds().Y+first.Bounds().H+16 {
		t.Fatal("narrow layout did not stack", first.Bounds(), second.Bounds())
	}
}

func TestHiddenRowItemsDoNotTakeSpace(t *testing.T) {
	t.Parallel()

	hidden := ui.NewButton("Hidden", nil)
	hidden.SetHidden(true)

	first, second := ui.NewButton("A", nil), ui.NewButton("B", nil)

	view := surface(t, ui.Row(first, hidden, second).WithStyle(ui.Style{Gap: 10}))
	if first.Bounds().W != 245 || second.Bounds().W != 245 || second.Bounds().X != 255 {
		t.Fatal("hidden item consumed row space", first.Bounds(), second.Bounds())
	}

	paint(t, view)
}
