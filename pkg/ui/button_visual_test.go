package ui_test

import (
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
)

func TestButtonVisualRejectsInvalidValuesAtomically(t *testing.T) {
	t.Parallel()

	button := ui.NewButton("Help", nil)

	visual := ui.ButtonVisual{Icon: "?", Side: ui.TrailingContent, Size: 20}
	if err := button.SetButtonVisual(visual); err != nil {
		t.Fatal(err)
	}

	for _, invalid := range []ui.ButtonVisual{
		{Icon: "?", Image: buttonImage(t)},
		{Icon: "\xff"},
		{Icon: "multi\nline"},
		{Side: ui.ContentSide(255)},
		{Size: -1},
		{Size: float32(math.NaN())},
		{Size: 257},
		{Gap: -1},
		{Image: &ui.ImageAsset{}},
	} {
		if err := button.SetButtonVisual(invalid); !errors.Is(err, ui.ErrBounds) || button.ButtonVisual() != visual {
			t.Fatal("invalid visual changed the button", invalid, button.ButtonVisual(), err)
		}
	}

	if err := ui.NewText("Text").SetButtonVisual(visual); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("non-button accepted button content", err)
	}
}

func TestCompactIconButtonsFitTheirGlyphsAndKeepNames(t *testing.T) {
	t.Parallel()

	for _, symbol := range []string{"+", "−", "?"} {
		t.Run(symbol, func(t *testing.T) {
			t.Parallel()

			button := ui.NewButton("", nil).WithStyle(ui.Style{Width: 44, Height: 42})
			if err := button.SetLabel("Named action", ""); err != nil {
				t.Fatal(err)
			}

			if err := button.SetButtonVisual(ui.ButtonVisual{Icon: symbol, Size: 24}); err != nil {
				t.Fatal(err)
			}

			view := surface(t, ui.Column(button))
			glyphs := 0

			for _, record := range buttonRecords(t, view) {
				if record.mode != 2 {
					continue
				}

				glyphs++

				if record.rect.X < record.clip.X || record.rect.Y < record.clip.Y ||
					record.rect.X+record.rect.W > record.clip.X+record.clip.W+0.001 ||
					record.rect.Y+record.rect.H > record.clip.Y+record.clip.H+0.001 {
					t.Fatal("compact icon glyph was cropped", record)
				}
			}

			if glyphs == 0 || view.Semantics()[1].Name != "Named action" {
				t.Fatal("icon-only action lost glyphs or its accessible name", glyphs, view.Semantics())
			}
		})
	}
}
