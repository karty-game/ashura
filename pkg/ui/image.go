package ui

import (
	"github.com/karty-game/ashura/internal/uipaint"
	"github.com/karty-game/ashura/pkg/texture"
)

// ImageAsset owns an immutable prepared page or a region of that page.
// Assets can be shared by buttons, tabs and images without copying their pixels.
type ImageAsset struct {
	page          *uipaint.Texture
	uv            Rect
	width, height int
}

// NewImageAsset copies GPU-ready straight-alpha RGBA8 pixels. It does not decode
// image files, resize images or build an atlas; those operations belong offline.
func NewImageAsset(width, height int, pixels []byte) (*ImageAsset, error) {
	return NewPreparedImageAsset(width, height, texture.RGBA8, pixels)
}

// NewPreparedImageAsset copies one GPU-ready RGBA8, BC1, BC7, ETC2RGB,
// ETC2RGBA or ASTC4x4 page. Block formats require four-texel aligned dimensions
// and enabled device support. Sampling retains the existing linear page semantics.
func NewPreparedImageAsset(width, height int, format texture.Format, pixels []byte) (*ImageAsset, error) {
	if !uipaint.ValidPage(width, height, format, len(pixels)) {
		return nil, ErrBounds
	}

	return &ImageAsset{
		page:   &uipaint.Texture{Width: width, Height: height, Format: format, Pixels: append([]byte(nil), pixels...)},
		uv:     Rect{W: 1, H: 1},
		width:  width,
		height: height,
	}, nil
}

func (asset *ImageAsset) Width() int  { return asset.width }
func (asset *ImageAsset) Height() int { return asset.height }

// Region references integer pixel bounds within this asset, retaining the same
// immutable page. Nested regions use coordinates relative to their parent region.
func (asset *ImageAsset) Region(bounds Rect) (*ImageAsset, error) {
	if !validImageAsset(asset) || !validImageRegion(bounds, asset.width, asset.height) {
		return nil, ErrBounds
	}

	return &ImageAsset{
		page: asset.page,
		uv: Rect{
			X: asset.uv.X + bounds.X/float32(asset.width)*asset.uv.W,
			Y: asset.uv.Y + bounds.Y/float32(asset.height)*asset.uv.H,
			W: bounds.W / float32(asset.width) * asset.uv.W,
			H: bounds.H / float32(asset.height) * asset.uv.H,
		},
		width:  int(bounds.W),
		height: int(bounds.H),
	}, nil
}

// NewImage preserves the convenience constructor for one prepared image node.
func NewImage(width, height int, pixels []byte) (*Node, error) {
	asset, err := NewImageAsset(width, height, pixels)
	if err != nil {
		return nil, err
	}

	return NewImageFromAsset(asset)
}

// NewImageFromAsset creates a decorative image sharing an immutable asset page.
func NewImageFromAsset(asset *ImageAsset) (*Node, error) {
	if !validImageAsset(asset) {
		return nil, ErrBounds
	}

	return &Node{kind: Image, image: asset.page, imageUV: asset.uv}, nil
}

func validImageAsset(asset *ImageAsset) bool {
	return asset != nil && asset.page != nil && asset.width > 0 && asset.height > 0
}

func validImageRegion(bounds Rect, width, height int) bool {
	for _, value := range []float32{bounds.X, bounds.Y, bounds.W, bounds.H} {
		if !finite(value) || value < 0 || value > 4096 || value != float32(int(value)) {
			return false
		}
	}

	return bounds.W > 0 && bounds.H > 0 && bounds.X+bounds.W <= float32(width) && bounds.Y+bounds.H <= float32(height)
}
