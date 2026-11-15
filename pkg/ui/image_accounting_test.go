package ui

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

func TestSharedImageAdmissionAndAtomicVisualReplacement(t *testing.T) {
	t.Parallel()

	asset, err := NewImageAsset(2, 1, make([]byte, 8))
	if err != nil {
		t.Fatal(err)
	}

	first, second := NewButton("First", nil), NewButton("Second", nil)
	for _, button := range []*Node{first, second} {
		if visualErr := button.SetButtonVisual(ButtonVisual{Image: asset}); visualErr != nil {
			t.Fatal(visualErr)
		}
	}

	// Resource admission does not need shaping tables or a GPU. The small font
	// page makes exact reference accounting visible without a large fixture.
	face := &Font{page: &uipaint.Texture{Width: 1, Height: 1, Pixels: make([]byte, 4)}}

	view, err := New(Column(first, second), face)
	if err != nil {
		t.Fatal(err)
	}

	if view.imageBytes != 8 || view.images[asset.page] != 2 || preparedImageBytes(view.root) != 8 {
		t.Fatal("shared asset was counted more than once", view.imageBytes, view.images)
	}

	// Exercise the admission boundary without allocating 128 MiB of image data.
	// imageBytes represents other already admitted pages plus the shared page.
	view.imageBytes = MaxImageBytes - len(face.page.Pixels)

	third := NewButton("Third", nil)
	if err := third.SetButtonVisual(ButtonVisual{Image: asset}); err != nil {
		t.Fatal(err)
	}

	if err := view.root.Add(third); err != nil || view.images[asset.page] != 3 {
		t.Fatal("adding another shared reference spent the image budget", err, view.images)
	}

	replacement, err := NewImageAsset(1, 1, make([]byte, 4))
	if err != nil {
		t.Fatal(err)
	}

	before := first.ButtonVisual()
	if err := first.SetButtonVisual(ButtonVisual{Image: replacement}); !errors.Is(err, ErrBounds) {
		t.Fatal("shared old page was incorrectly reclaimed for replacement", err)
	}

	if first.ButtonVisual() != before || view.images[asset.page] != 3 || view.images[replacement.page] != 0 {
		t.Fatal("rejected replacement mutated content or ownership", first.ButtonVisual(), view.images)
	}

	second.Remove()
	third.Remove()

	if err := first.SetButtonVisual(ButtonVisual{Image: replacement}); err != nil {
		t.Fatal("exclusive old page was not reclaimed for replacement", err)
	}

	if view.images[asset.page] != 0 || view.images[replacement.page] != 1 || view.imageBytes != MaxImageBytes-8 {
		t.Fatal("replacement did not transfer resource ownership exactly", view.imageBytes, view.images)
	}
}

func TestImageRegionBoundsAndNestedCoordinates(t *testing.T) {
	t.Parallel()

	asset, err := NewImageAsset(8, 4, make([]byte, 8*4*4))
	if err != nil {
		t.Fatal(err)
	}

	region, err := asset.Region(Rect{X: 2, Y: 1, W: 4, H: 2})
	if err != nil {
		t.Fatal(err)
	}

	nested, err := region.Region(Rect{X: 1, W: 2, H: 1})
	if err != nil {
		t.Fatal(err)
	}

	if nested.Width() != 2 || nested.Height() != 1 || nested.page != asset.page ||
		nested.uv != (Rect{X: 0.375, Y: 0.25, W: 0.25, H: 0.25}) {
		t.Fatal("nested region did not preserve page-relative coordinates", nested)
	}

	for _, bounds := range []Rect{{W: 0, H: 1}, {X: -1, W: 1, H: 1}, {W: 9, H: 1}, {X: 0.5, W: 1, H: 1}, {Y: 4, W: 1, H: 1}} {
		if _, err := asset.Region(bounds); !errors.Is(err, ErrBounds) {
			t.Fatal("invalid atlas region was admitted", bounds, err)
		}
	}

	if _, err := NewImageFromAsset(&ImageAsset{}); !errors.Is(err, ErrBounds) {
		t.Fatal("zero-value image asset was admitted", err)
	}
}
