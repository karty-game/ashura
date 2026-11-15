package ui_test

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func panelReviewAsset(t *testing.T, width, height int) *ui.ImageAsset {
	t.Helper()

	pixels := make([]byte, width*height*4)
	for index := range pixels {
		pixels[index] = 255
	}

	asset, err := ui.NewImageAsset(width, height, pixels)
	if err != nil {
		t.Fatal(err)
	}

	return asset
}

func panelReviewPieces(t *testing.T, view *ui.View) []submittedButtonRecord {
	t.Helper()

	pieces := []submittedButtonRecord{}

	for _, record := range buttonRecords(t, view) {
		if record.mode == 7 {
			pieces = append(pieces, record)
		}
	}

	return pieces
}

func TestPanelReviewNineSliceAtlasAndResize(t *testing.T) {
	t.Parallel()

	atlas := panelReviewAsset(t, 32, 32)

	asset, err := atlas.Region(ui.Rect{X: 8, Y: 8, W: 16, H: 16})
	if err != nil {
		t.Fatal(err)
	}

	panel := ui.Column().WithStyle(ui.Style{Width: 120, Height: 90})

	slice := &ui.NineSlice{Left: 4, Top: 3, Right: 2, Bottom: 5, Scale: 1.5}
	if err := panel.SetPanelBackground(ui.PanelBackground{Image: asset, Slice: slice}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(panel))

	pieces := panelReviewPieces(t, view)
	if len(pieces) != 9 {
		t.Fatal("nine-slice did not submit nine pieces", len(pieces))
	}

	first := pieces[0]
	if first.rect.W != 6 || first.rect.H != 4.5 || first.uv != (ui.Rect{X: 0.25, Y: 0.25, W: 0.125, H: 0.09375}) {
		t.Fatal("atlas slice lost source UVs or scaled corner dimensions", first)
	}

	for _, piece := range pieces {
		if piece.texture != first.texture {
			t.Fatal("slices did not share one atlas texture")
		}
	}

	if err := panel.SetStyle(ui.Style{Width: 220, Height: 150}); err != nil {
		t.Fatal(err)
	}

	resized := panelReviewPieces(t, view)
	if len(resized) != 9 || resized[0].rect.W != first.rect.W || resized[0].rect.H != first.rect.H || resized[0].uv != first.uv {
		t.Fatal("resize stretched fixed source corners", first, resized)
	}

	area := float32(0)
	for _, piece := range resized {
		area += piece.rect.W * piece.rect.H
	}

	if area != 220*150 {
		t.Fatal("slice pieces do not cover the panel exactly", area)
	}

	if err := panel.SetStyle(ui.Style{Width: 6, Height: 4}); err != nil {
		t.Fatal(err)
	}

	small := panelReviewPieces(t, view)
	area = 0

	for _, piece := range small {
		if piece.rect.W <= 0 || piece.rect.H <= 0 || piece.rect.X < panel.Bounds().X || piece.rect.Y < panel.Bounds().Y ||
			piece.rect.X+piece.rect.W > panel.Bounds().X+6.001 || piece.rect.Y+piece.rect.H > panel.Bounds().Y+4.001 {
			t.Fatal("small-panel pieces overlap or escape bounds", piece)
		}

		area += piece.rect.W * piece.rect.H
	}

	if len(small) != 4 || math.Abs(float64(area-24)) > 0.001 {
		t.Fatal("undersized panel failed to compress opposing borders", len(small), area)
	}
}

func TestPanelReviewImageFitAndBorderOnly(t *testing.T) {
	t.Parallel()

	asset := panelReviewAsset(t, 20, 10)

	panel := ui.Column().WithStyle(ui.Style{Width: 100, Height: 100})
	if err := panel.SetPanelBackground(ui.PanelBackground{Image: asset, Fit: ui.ContainImage}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(panel))

	pieces := panelReviewPieces(t, view)
	if len(pieces) != 1 || pieces[0].rect != (ui.Rect{W: 100, Y: 25, H: 50}) {
		t.Fatal("contain failed to preserve source aspect ratio", pieces)
	}

	if err := panel.SetPanelBackground(ui.PanelBackground{Image: asset, Fit: ui.CoverImage}); err != nil {
		t.Fatal(err)
	}

	pieces = panelReviewPieces(t, view)
	if len(pieces) != 1 || pieces[0].rect != panel.Bounds() || pieces[0].uv != (ui.Rect{X: 0.25, W: 0.5, H: 1}) {
		t.Fatal("cover did not crop UVs symmetrically", pieces)
	}

	if err := panel.SetPanelBackground(
		ui.PanelBackground{Image: asset, Slice: &ui.NineSlice{Left: 2, Top: 2, Right: 2, Bottom: 2, BorderOnly: true}},
	); err != nil {
		t.Fatal(err)
	}

	if pieces = panelReviewPieces(t, view); len(pieces) != 8 {
		t.Fatal("border-only image painted its center", len(pieces))
	}

	if err := panel.SetPanelBackground(ui.PanelBackground{Image: asset, Slice: &ui.NineSlice{}}); err != nil {
		t.Fatal(err)
	}

	if pieces = panelReviewPieces(t, view); len(pieces) != 1 || pieces[0].rect != panel.Bounds() {
		t.Fatal("zero insets did not reduce to one center piece", pieces)
	}
}

func TestPanelReviewDescriptorOwnershipAndRejection(t *testing.T) {
	t.Parallel()

	asset := panelReviewAsset(t, 16, 16)
	panel := ui.Column()
	slice := &ui.NineSlice{Left: 3, Top: 3, Right: 3, Bottom: 3}

	background := ui.PanelBackground{Image: asset, Slice: slice}
	if err := panel.SetPanelBackground(background); err != nil {
		t.Fatal(err)
	}

	slice.Left = 100
	getter := panel.PanelBackground()

	getter.Slice.Right = 100
	if panel.PanelBackground().Slice.Left != 3 || panel.PanelBackground().Slice.Right != 3 {
		t.Fatal("background descriptor retained caller or getter slice storage")
	}

	before := panel.PanelBackground()
	for _, invalid := range []ui.PanelBackground{
		{Fit: ui.CoverImage},
		{Image: &ui.ImageAsset{}},
		{Image: asset, Fit: ui.ImageFit(255)},
		{Image: asset, Tint: ui.Color{float32(math.NaN()), 1, 1, 1}},
		{Image: asset, Fit: ui.CoverImage, Slice: &ui.NineSlice{Left: 2}},
		{Image: asset, Slice: &ui.NineSlice{Left: 16}},
		{Image: asset, Slice: &ui.NineSlice{Top: -1}},
		{Image: asset, Slice: &ui.NineSlice{Left: 0.5}},
		{Image: asset, Slice: &ui.NineSlice{Scale: 65}},
	} {
		if err := panel.SetPanelBackground(invalid); !errors.Is(err, ui.ErrBounds) {
			t.Fatal("invalid panel descriptor was admitted", invalid, err)
		}

		after := panel.PanelBackground()
		if after.Image != before.Image || after.Fit != before.Fit || after.Tint != before.Tint || *after.Slice != *before.Slice {
			t.Fatal("rejected descriptor changed panel state", before, after)
		}
	}

	if err := ui.NewButton("Button", nil).SetPanelBackground(before); !errors.Is(err, ui.ErrBounds) {
		t.Fatal("non-panel accepted panel background", err)
	}
}

func TestPanelReviewRetainsInteractionPaintAndBorrowedFrame(t *testing.T) {
	t.Parallel()

	activations := 0
	button := ui.NewButton("Inside", func() { activations++ })
	panel := ui.Column(button).
		WithStyle(ui.Style{Padding: 12, Height: 120, Fill: ui.Hex(0x13212c), Border: ui.Hex(0x35516a), BorderWidth: 1})

	asset := panelReviewAsset(t, 8, 8)
	if err := panel.SetPanelBackground(ui.PanelBackground{Image: asset}); err != nil {
		t.Fatal(err)
	}

	view := surface(t, ui.Column(panel))
	view.SetReducedMotion(true)
	click(view, button)

	if activations != 1 || view.Focused() != button || len(view.Semantics()) != 3 {
		t.Fatal("decorative background changed hit testing or semantics", activations, view.Focused(), view.Semantics())
	}

	frame, err := view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	previous := append([]uipaint.Batch(nil), frame.Batches...)
	stats := view.Stats()

	if err := panel.SetPanelBackground(ui.PanelBackground{}); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(previous, frame.Batches) {
		t.Fatal("background setter mutated a previously borrowed frame")
	}

	if pieces := panelReviewPieces(
		t,
		view,
	); len(pieces) != 0 || view.Stats().Measures != stats.Measures || view.Stats().Shapes != stats.Shapes ||
		view.Stats().Paints != stats.Paints+1 {
		t.Fatal("clearing background remeasured or repainted unrelated content", pieces, stats, view.Stats())
	}

	frame, err = view.Frame(time.Second)
	if err != nil {
		t.Fatal(err)
	}

	for _, batch := range frame.Batches[:cap(frame.Batches)] {
		for _, old := range previous {
			if old.Texture != nil && old.Texture != frame.Batches[0].Texture && old.Texture == batch.Texture {
				t.Fatal("cleared background remained referenced by output batch storage")
			}
		}
	}

	idle := view.Stats()
	if _, err := view.Frame(2 * time.Second); err != nil || view.Stats() != idle {
		t.Fatal("cleared panel did not return to retained idle", err, idle, view.Stats())
	}
}
