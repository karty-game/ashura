package ui

import (
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/uipaint"
)

func panelAsset(t *testing.T, width, height int) *ImageAsset {
	t.Helper()

	asset, err := NewImageAsset(width, height, make([]byte, width*height*4))
	if err != nil {
		t.Fatal(err)
	}

	return asset
}

func TestPanelBackgroundFitsAndPreservesWholePanelMask(t *testing.T) {
	t.Parallel()

	asset := panelAsset(t, 8, 4)

	for _, example := range []struct {
		fit    ImageFit
		bounds Rect
		uv     Rect
	}{
		{StretchImage, Rect{X: 10, Y: 20, W: 120, H: 120}, Rect{W: 1, H: 1}},
		{ContainImage, Rect{X: 10, Y: 50, W: 120, H: 60}, Rect{W: 1, H: 1}},
		{CoverImage, Rect{X: 10, Y: 20, W: 120, H: 120}, Rect{X: 0.25, W: 0.5, H: 1}},
	} {
		view := &View{theme: DarkTheme()}
		panel := NewPanel().WithStyle(Style{Radius: 9})

		panel.rect, panel.clip = Rect{X: 10, Y: 20, W: 120, H: 120}, Rect{W: 500, H: 400}
		if err := panel.SetPanelBackground(PanelBackground{Image: asset, Fit: example.fit}); err != nil {
			t.Fatal(err)
		}

		view.paintPanelBackground(panel)

		if len(panel.paint) != 1 {
			t.Fatal("plain image did not use one quad", len(panel.paint))
		}

		record := panel.paint[0]
		if record.rect != example.bounds || record.uv != example.uv || record.border != Color(panel.rect.Array()) ||
			record.radius != 9 || record.mode != 7 || record.texture != asset.page || record.color != (Color{1, 1, 1, 1}) {
			t.Fatal("fit or whole-panel clipping payload changed", example.fit, record)
		}
	}
}

func TestNineSliceUsesNestedAtlasCutsAndSmallBounds(t *testing.T) {
	t.Parallel()

	asset := panelAsset(t, 16, 16)

	region, err := asset.Region(Rect{X: 4, Y: 4, W: 8, H: 8})
	if err != nil {
		t.Fatal(err)
	}

	view := &View{theme: DarkTheme()}
	panel := NewPanel()
	panel.rect, panel.clip = Rect{X: 10, Y: 20, W: 20, H: 10}, Rect{W: 500, H: 400}

	background := PanelBackground{Image: region, Slice: &NineSlice{Left: 2, Top: 2, Right: 2, Bottom: 2}}
	if err := panel.SetPanelBackground(background); err != nil {
		t.Fatal(err)
	}

	view.paintPanelBackground(panel)

	if len(panel.paint) != 9 || panel.paint[0].rect != (Rect{X: 10, Y: 20, W: 2, H: 2}) ||
		panel.paint[0].uv != (Rect{X: 0.25, Y: 0.25, W: 0.125, H: 0.125}) || panel.paint[8].rect != (Rect{X: 28, Y: 28, W: 2, H: 2}) ||
		panel.paint[8].uv != (Rect{X: 0.625, Y: 0.625, W: 0.125, H: 0.125}) {
		t.Fatal("nine-slice lost source-region corners", panel.paint)
	}

	background.Slice.BorderOnly = true
	if err := panel.SetPanelBackground(background); err != nil {
		t.Fatal(err)
	}

	panel.paint = panel.paint[:0]
	view.paintPanelBackground(panel)

	if len(panel.paint) != 8 {
		t.Fatal("border-only image painted its center", len(panel.paint))
	}

	panel.rect = Rect{X: 10, Y: 20, W: 3, H: 2}
	panel.paint = panel.paint[:0]
	view.paintPanelBackground(panel)

	if len(panel.paint) != 4 {
		t.Fatal("small target should contain four fitted corners", panel.paint)
	}

	for _, record := range panel.paint {
		if record.rect.W <= 0 || record.rect.H <= 0 || record.rect.X < panel.rect.X || record.rect.Y < panel.rect.Y ||
			record.rect.X+record.rect.W > panel.rect.X+panel.rect.W || record.rect.Y+record.rect.H > panel.rect.Y+panel.rect.H {
			t.Fatal("small panel produced overlapping/outside geometry", record.rect)
		}
	}
}

func TestPanelBackgroundDescriptorsAreCopiedAndInvalidUpdatesAtomic(t *testing.T) {
	t.Parallel()

	asset := panelAsset(t, 8, 8)
	panel := NewPanel()

	background := PanelBackground{Image: asset, Slice: &NineSlice{Left: 2, Top: 2, Right: 2, Bottom: 2}}
	if err := panel.SetPanelBackground(background); err != nil {
		t.Fatal(err)
	}

	background.Slice.Left = 7
	descriptor := panel.PanelBackground()

	descriptor.Slice.Right = 7
	if panel.panelBackground.Slice.Left != 2 || panel.panelBackground.Slice.Right != 2 {
		t.Fatal("descriptor retained mutable slice metadata")
	}

	before := panel.PanelBackground()
	for _, invalid := range []PanelBackground{
		{Image: asset, Fit: ImageFit(255)},
		{Image: asset, Tint: Color{2, 0, 0, 1}},
		{Image: asset, Slice: &NineSlice{Left: float32(math.NaN())}},
		{Image: asset, Slice: &NineSlice{Left: 0.5}},
		{Image: asset, Slice: &NineSlice{Left: 4, Right: 4}},
		{Image: asset, Slice: &NineSlice{Top: 4, Bottom: 4}},
		{Image: asset, Slice: &NineSlice{Scale: -1}},
		{Image: asset, Slice: &NineSlice{Scale: 65}},
		{Image: asset, Fit: CoverImage, Slice: &NineSlice{}},
		{Slice: &NineSlice{}},
		{Image: &ImageAsset{}},
	} {
		if err := panel.SetPanelBackground(invalid); !errors.Is(err, ErrBounds) || !equalPanelBackground(before, panel.PanelBackground()) {
			t.Fatal("invalid descriptor changed the panel", invalid, err)
		}
	}

	if err := NewButton("Action", nil).SetPanelBackground(before); !errors.Is(err, ErrBounds) {
		t.Fatal("button accepted a panel background", err)
	}
}

func TestPanelBackgroundResourceReplacementAndHiddenClear(t *testing.T) {
	t.Parallel()

	asset, replacement := panelAsset(t, 2, 1), panelAsset(t, 1, 1)

	panel, sibling := NewPanel(), NewPanel()
	for _, node := range []*Node{panel, sibling} {
		if err := node.SetPanelBackground(PanelBackground{Image: asset}); err != nil {
			t.Fatal(err)
		}
	}

	face := &Font{page: &uipaint.Texture{Width: 1, Height: 1, Pixels: make([]byte, 4)}}

	view, err := New(Column(panel, sibling), face)
	if err != nil {
		t.Fatal(err)
	}

	if view.imageBytes != 8 || view.images[asset.page] != 2 || preparedImageBytes(view.root) != 8 {
		t.Fatal("shared panel pages counted more than once", view.imageBytes, view.images)
	}

	view.imageBytes = MaxImageBytes - len(face.page.Pixels)
	if err := panel.SetPanelBackground(PanelBackground{Image: replacement}); !errors.Is(err, ErrBounds) ||
		panel.PanelBackground().Image != asset || view.images[asset.page] != 2 {
		t.Fatal("rejected replacement changed the retained resource", err, view.images)
	}

	sibling.Remove()

	if err := panel.SetPanelBackground(PanelBackground{Image: replacement}); err != nil {
		t.Fatal(err)
	}

	if view.images[asset.page] != 0 || view.images[replacement.page] != 1 || view.imageBytes != MaxImageBytes-8 {
		t.Fatal("exclusive replacement did not release old resource", view.imageBytes, view.images)
	}

	panel.hidden = true
	panel.paint = append(panel.paint, paintRecord{texture: replacement.page}, paintRecord{texture: replacement.page})
	panel.batches = append(panel.batches, uipaint.Batch{Texture: replacement.page}, uipaint.Batch{Texture: replacement.page})
	panel.paint = panel.paint[:1]
	panel.batches = panel.batches[:1]

	view.frame.Batches = append(view.frame.Batches, uipaint.Batch{Texture: replacement.page})

	if err := panel.SetPanelBackground(PanelBackground{}); err != nil {
		t.Fatal(err)
	}

	if panel.paint[0].texture != nil || panel.batches[0].Texture != nil || view.images[replacement.page] != 0 ||
		view.frame.Batches[0].Texture != replacement.page {
		t.Fatal("hidden clear retained paint references or mutated borrowed frame")
	}

	for _, record := range panel.paint[:cap(panel.paint)] {
		if record.texture != nil {
			t.Fatal("hidden clear kept an unused paint reference")
		}
	}

	for _, batch := range panel.batches[:cap(panel.batches)] {
		if batch.Texture != nil {
			t.Fatal("hidden clear kept an unused batch reference")
		}
	}
}
