package ui

import "github.com/karty-game/karty-webgpu/internal/uipaint"

// ImageFit controls how an unsliced panel image fits its bounds.
type ImageFit uint8

const (
	StretchImage ImageFit = iota
	ContainImage
	CoverImage
)

// NineSlice preserves corner areas while stretching the center and edges.
// Insets are integer source pixels relative to ImageAsset, including atlas regions.
// Scale defaults to one logical pixel per source pixel. BorderOnly omits the center.
type NineSlice struct {
	Left, Top, Right, Bottom float32
	Scale                    float32
	BorderOnly               bool
}

// PanelBackground decorates a panel without changing layout or hit testing.
// Tint defaults to white. Slice requires StretchImage; a zero value clears the image.
type PanelBackground struct {
	Image *ImageAsset
	Fit   ImageFit
	Tint  Color
	Slice *NineSlice
}

// SetPanelBackground copies and validates a complete descriptor atomically.
// Images remain shared immutable assets; only affected panel paint is invalidated.
func (node *Node) SetPanelBackground(background PanelBackground) error {
	if node.kind != Panel || validatePanelBackground(background) != nil {
		return ErrBounds
	}

	if equalPanelBackground(node.panelBackground, background) {
		return nil
	}

	oldPage, newPage := panelBackgroundImage(node.panelBackground), panelBackgroundImage(background)
	if node.view != nil {
		if !node.view.canReplaceImage(oldPage, newPage) {
			return ErrBounds
		}

		node.view.releaseImage(oldPage)
		node.view.retainImage(newPage)
	}

	if oldPage != newPage {
		node.clearPanelImageReferences(oldPage)
	}

	node.panelBackground = copyPanelBackground(background)
	node.damage(false)

	return nil
}

func (node *Node) PanelBackground() PanelBackground {
	return copyPanelBackground(node.panelBackground)
}

func copyPanelBackground(background PanelBackground) PanelBackground {
	if background.Slice != nil {
		slice := *background.Slice
		background.Slice = &slice
	}

	return background
}

func equalPanelBackground(first, last PanelBackground) bool {
	if first.Image != last.Image || first.Fit != last.Fit || first.Tint != last.Tint {
		return false
	}

	if first.Slice == nil || last.Slice == nil {
		return first.Slice == last.Slice
	}

	return *first.Slice == *last.Slice
}

func validatePanelBackground(background PanelBackground) error {
	if background.Image == nil {
		if background != (PanelBackground{}) {
			return ErrBounds
		}

		return nil
	}

	if !validImageAsset(background.Image) || background.Fit > CoverImage || !validBackgroundTint(background.Tint) {
		return ErrBounds
	}

	if background.Slice != nil {
		if background.Fit != StretchImage || !validNineSlice(background.Image, *background.Slice) {
			return ErrBounds
		}
	}

	return nil
}

func validNineSlice(image *ImageAsset, slice NineSlice) bool {
	for _, inset := range []float32{slice.Left, slice.Top, slice.Right, slice.Bottom} {
		if !finite(inset) || inset < 0 || inset > 4096 || inset != float32(int(inset)) {
			return false
		}
	}

	return finite(slice.Scale) && slice.Scale >= 0 && slice.Scale <= 64 &&
		slice.Left+slice.Right < float32(image.width) && slice.Top+slice.Bottom < float32(image.height)
}

func validBackgroundTint(tint Color) bool {
	for _, channel := range tint {
		if !finite(channel) || channel < 0 || channel > 1 {
			return false
		}
	}

	return true
}

func panelBackgroundImage(background PanelBackground) *uipaint.Texture {
	if background.Image != nil {
		return background.Image.page
	}

	return nil
}

// The previously borrowed Frame owns its own batch values until the next Frame.
// Drop only the panel's retained references, including when it is hidden.
func (node *Node) clearPanelImageReferences(page *uipaint.Texture) {
	if page == nil {
		return
	}

	paint := node.paint[:cap(node.paint)]
	for index := range paint {
		if paint[index].texture == page {
			paint[index].texture = nil
		}
	}

	batches := node.batches[:cap(node.batches)]
	for index := range batches {
		if batches[index].Texture == page {
			batches[index].Texture = nil
		}
	}
}
