package ui_test

import (
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func TestButtonVisualAlignsWithLabelGlyphs(t *testing.T) {
	t.Parallel()

	for _, example := range []struct {
		name, text string
		visual     ui.ButtonVisual
		fontSize   float32
	}{
		{"leading symbol", "Equip", ui.ButtonVisual{Icon: "+", Size: 20}, 16},
		{"trailing symbol", "Continue", ui.ButtonVisual{Icon: "›", Side: ui.TrailingContent, Size: 24}, 16},
		{"leading image", "Buy", ui.ButtonVisual{Image: buttonImage(t), Size: 30}, 16},
		{"trailing image", "Craft", ui.ButtonVisual{Image: buttonImage(t), Side: ui.TrailingContent, Size: 30}, 16},
		{"small text", "Small", ui.ButtonVisual{Icon: "+", Size: 20}, 12},
		{"large descender text", "Apply", ui.ButtonVisual{Icon: "?", Size: 20}, 24},
	} {
		t.Run(example.name, func(t *testing.T) {
			t.Parallel()

			button := ui.NewButton(example.text, nil).WithStyle(ui.Style{Height: 64, FontSize: example.fontSize})
			if err := button.SetButtonVisual(example.visual); err != nil {
				t.Fatal(err)
			}

			view := surface(t, ui.Column(button))
			records := buttonRecords(t, view)
			glyphs := []ui.Rect{}

			for _, record := range records {
				if record.mode == 2 {
					glyphs = append(glyphs, record.rect)
				}
			}

			decoration := records[len(records)-1].rect
			if example.visual.Image == nil {
				glyphs = glyphs[:len(glyphs)-1]
			}

			labelCenter := glyphVerticalCenter(t, glyphs)

			visualCenter := decoration.Y + decoration.H/2
			if math.Abs(float64(labelCenter-visualCenter)) > 0.001 {
				t.Fatal("button decoration is not aligned with label ink", labelCenter, visualCenter)
			}
		})
	}
}

func TestButtonVisualWithoutLabelRemainsCentered(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "   "} {
		button := ui.NewButton(text, nil).WithStyle(ui.Style{Width: 44, Height: 42})
		if err := button.SetButtonVisual(ui.ButtonVisual{Icon: "?", Size: 20}); err != nil {
			t.Fatal(err)
		}

		view := surface(t, ui.Column(button))
		records := buttonRecords(t, view)
		decoration := records[len(records)-1].rect

		center := button.Bounds().Y + button.Bounds().H/2
		if math.Abs(float64(decoration.Y+decoration.H/2-center)) > 0.001 {
			t.Fatal("visual-only button lost its centered decoration", text, decoration, center)
		}
	}
}

func glyphVerticalCenter(t *testing.T, glyphs []ui.Rect) float32 {
	t.Helper()

	if len(glyphs) == 0 {
		t.Fatal("missing submitted label glyphs")
	}

	top, bottom := glyphs[0].Y, glyphs[0].Y+glyphs[0].H
	for _, glyph := range glyphs[1:] {
		top, bottom = min(top, glyph.Y), max(bottom, glyph.Y+glyph.H)
	}

	return (top + bottom) / 2
}
