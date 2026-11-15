package ui

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

func TestOverlaySharesImageAdmissionAndReleasesReferences(t *testing.T) {
	t.Parallel()

	asset, err := NewImageAsset(2, 1, make([]byte, 8))
	if err != nil {
		t.Fatal(err)
	}

	font := &Font{page: &uipaint.Texture{Width: 1, Height: 1, Pixels: make([]byte, 4)}}
	button := NewButton("Root", nil).WithButtonVisual(ButtonVisual{Image: asset})

	view, err := New(Column(button), font)
	if err != nil {
		t.Fatal(err)
	}

	view.imageBytes = MaxImageBytes - len(font.page.Pixels)
	overlay := NewButton("Overlay", nil).WithButtonVisual(ButtonVisual{Image: asset})

	remove, err := view.AddOverlay(overlay, OverlayOptions{})
	if err != nil || view.images[asset.page] != 2 {
		t.Fatal("overlay duplicated the existing image page", err, view.images)
	}

	other, err := NewImageAsset(1, 1, make([]byte, 4))
	if err != nil {
		t.Fatal(err)
	}

	rejected := NewButton("Rejected", nil).WithButtonVisual(ButtonVisual{Image: other})
	if _, err := view.AddOverlay(rejected, OverlayOptions{}); !errors.Is(err, ErrBounds) || rejected.view != nil {
		t.Fatal("overlay exceeded image bounds or retained rejected ownership", err)
	}

	remove()
	remove()

	if view.images[asset.page] != 1 || len(view.overlays) != 0 || len(view.nodes) != 2 {
		t.Fatal("overlay removal retained pages/nodes", view.images, view.nodes)
	}
}

func TestOverlayAdmissionUsesSharedNodeBudget(t *testing.T) {
	t.Parallel()

	children := make([]*Node, MaxNodes-1)
	for index := range children {
		children[index] = NewText("Bounded")
	}

	font := &Font{page: &uipaint.Texture{Width: 1, Height: 1, Pixels: make([]byte, 4)}}

	view, err := New(Column(children...), font)
	if err != nil {
		t.Fatal(err)
	}

	root := Column()
	if _, err := view.AddOverlay(root, OverlayOptions{}); !errors.Is(err, ErrBounds) || root.view != nil || len(view.nodes) != MaxNodes {
		t.Fatal("overlay bypassed the View node budget", err, len(view.nodes))
	}
}
