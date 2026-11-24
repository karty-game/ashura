// Package draw2d provides retained pixel-coordinate sprites from prepared images.
package draw2d

import (
	"errors"

	"github.com/karty-game/ashura/internal/spritepaint"
	"github.com/karty-game/ashura/internal/uipaint"
	"github.com/karty-game/ashura/pkg/texture"
)

const (
	MaxSprites      = 65536
	MaxImageBytes   = 128 << 20
	MaxImageSize    = 4096
	MaxPages        = 256
	MaxViewportSize = 16384
	// MaxTransformValue bounds position, scale and pivot magnitudes, keeping
	// composed float32 geometry finite for the largest admitted image.
	MaxTransformValue float32 = 1 << 24
)

var ErrImage = errors.New("draw2d: invalid prepared image (maximum side 4096)")

// Image is an immutable source region sharing one prepared straight-alpha sRGB page.
type Image struct {
	page                *spritepaint.Texture
	x, y, width, height int
}

// NewImage admits already prepared pixels, copying them once. It performs no decoding.
func NewImage(width, height int, pixels []byte) (*Image, error) {
	return NewPreparedImage(width, height, texture.RGBA8, pixels)
}

// NewPreparedImage copies a single GPU-ready RGBA8, BC1, BC7, ETC2RGB,
// ETC2RGBA or ASTC4x4 page. Block formats require four-texel aligned dimensions
// and enabled device support. No decoding or runtime format conversion occurs.
func NewPreparedImage(width, height int, format texture.Format, pixels []byte) (*Image, error) {
	if !uipaint.ValidPage(width, height, format, len(pixels)) {
		return nil, ErrImage
	}

	return &Image{
		page:   &spritepaint.Texture{Width: width, Height: height, Format: format, Pixels: append([]byte(nil), pixels...)},
		width:  width,
		height: height,
	}, nil
}

// Region references half-open integer bounds relative to this image.
func (image *Image) Region(left, top, width, height int) (*Image, error) {
	if !validImage(image) || left < 0 || top < 0 || width < 1 || height < 1 || left > image.width || top > image.height ||
		width > image.width-left ||
		height > image.height-top {
		return nil, ErrImage
	}

	return &Image{page: image.page, x: image.x + left, y: image.y + top, width: width, height: height}, nil
}

func (image *Image) Width() int  { return image.width }
func (image *Image) Height() int { return image.height }

func validImage(image *Image) bool {
	return image != nil && image.page != nil && image.width > 0 && image.height > 0
}
