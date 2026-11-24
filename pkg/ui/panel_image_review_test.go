package ui_test

import (
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/uipaint"
	"github.com/karty-game/ashura/pkg/ui"
)

func TestPanelImageReviewComposesSharedFrameAndPhotoBehindInput(t *testing.T) {
	t.Parallel()

	atlas := panelReviewAsset(t, 32, 16)

	frameArt, err := atlas.Region(ui.Rect{W: 16, H: 16})
	if err != nil {
		t.Fatal(err)
	}

	photoArt, err := atlas.Region(ui.Rect{X: 16, W: 16, H: 8})
	if err != nil {
		t.Fatal(err)
	}

	input := ui.NewInput("Note", "Retained note", "", nil)
	photo := ui.Column(input).WithStyle(ui.Style{Grow: 1, Padding: 8, Fill: ui.Hex(0x152536)})
	background := ui.PanelBackground{Image: photoArt, Fit: ui.CoverImage}

	if err = photo.SetPanelBackground(background); err != nil {
		t.Fatal(err)
	}

	frame := ui.Column(photo).WithStyle(ui.Style{Width: 200, Height: 140, Padding: 8, Fill: ui.Hex(0x14232b)})

	if err = frame.SetPanelBackground(ui.PanelBackground{
		Image: frameArt, Slice: &ui.NineSlice{Left: 3, Top: 3, Right: 3, Bottom: 3, BorderOnly: true},
	}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(frame))
	view.SetReducedMotion(true)
	view.SetFocus(input)
	paint(t, view)

	assertComposedPanelImageRecords(t, view, photo, input)

	before, bounds := view.Stats(), input.Bounds()

	if err = photo.SetPanelBackground(ui.PanelBackground{}); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if len(panelReviewPieces(t, view)) != 8 || view.Focused() != input || input.Text() != "Retained note" || input.Bounds() != bounds {
		t.Fatal("clearing only the photo removed the frame or changed its input")
	}

	if view.Stats().Measures != before.Measures || view.Stats().Shapes != before.Shapes || view.Stats().Paints != before.Paints+1 {
		t.Fatal("photo visibility changed layout/text or repainted unrelated controls", before, view.Stats())
	}

	if err = photo.SetPanelBackground(background); err != nil {
		t.Fatal(err)
	}

	paint(t, view)

	if len(panelReviewPieces(t, view)) != 9 || view.Focused() != input || view.Editing().Text != "Retained note" {
		t.Fatal("photo restoration lost image, focus or editing state")
	}

	idle := view.Stats()

	if _, err = view.Frame(time.Second); err != nil || view.Stats() != idle {
		t.Fatal("composed photo/frame failed to return to retained idle", err, idle, view.Stats())
	}
}

func assertComposedPanelImageRecords(t *testing.T, view *ui.View, photo, input *ui.Node) {
	t.Helper()

	records := buttonRecords(t, view)
	pieces, photoIndex, inputIndex := 0, -1, -1

	var page *uipaint.Texture

	for index, record := range records {
		if record.mode == 0 && record.rect == input.Bounds() {
			inputIndex = index
		}

		if record.mode != 7 {
			continue
		}

		pieces++

		if page == nil {
			page = record.texture
		}

		if record.texture != page {
			t.Fatal("composed frame/photo copied their shared atlas page")
		}

		if record.rect != photo.Bounds() {
			if photoIndex >= 0 {
				t.Fatal("frame decoration was reordered above its inset photo")
			}

			continue
		}

		photoIndex = index

		if record.uv.X <= 0.5 || record.uv.W >= 0.5 || record.uv.H != 0.5 {
			t.Fatal("cover image lost its generated subregion or centered crop", record)
		}
	}

	if pieces != 9 || photoIndex < 0 || inputIndex <= photoIndex || photo.Bounds() != (ui.Rect{X: 8, Y: 8, W: 184, H: 124}) {
		t.Fatal("frame, inset photo and input were not composed in painter order", pieces, photoIndex, inputIndex, photo.Bounds())
	}
}
