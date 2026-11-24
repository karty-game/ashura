// Package texturedata owns immutable GPU-ready texture payloads.
// It imports no image decoders or container formats.
package texturedata

import "errors"

const (
	maxDimension       = 4096
	blockDimension     = 4
	astcBlockDimension = 6
	blockBytes         = 8
	wideBlockBytes     = 16
	// Channels is the prepared RGBA8 channel count.
	Channels = 4
)

// ColorSpace describes how the texture's RGB channels are interpreted.
type ColorSpace uint8

const (
	// SRGB decodes color channels to linear values during GPU sampling.
	SRGB ColorSpace = iota
	// Linear preserves numeric channels, including tangent-space normal maps.
	Linear
)

// Data describes prepared GPU mip levels, largest first. Each level contains
// tightly packed pixel or block rows. Zero Format selects RGBA8. No decoding, resizing,
// channel conversion or mip generation is performed by the engine.
type Data struct {
	Width, Height int
	ColorSpace    ColorSpace
	Format        Format
	Levels        [][]byte
}

// Texture is an immutable, App-owned texture handle. Unloading invalidates it.
type Texture struct {
	width, height int
	bytes         uint64
	levels        [][]byte
	space         ColorSpace
	format        Format
	released      bool
	storage       bool
}

// Size returns the original texture dimensions.
func (texture *Texture) Size() (int, int) { return texture.width, texture.height }

var (
	errDimensions = errors.New("texture dimensions must be between 1 and 4096")
	errColorSpace = errors.New("invalid texture color space")
	errMipLevels  = errors.New("texture must contain valid tightly packed RGBA8 mip levels")
)

// Create validates and copies already prepared GPU bytes into an owned handle.
func Create(data Data) (*Texture, error) {
	texture, err := FromOwned(data)
	if err != nil {
		return nil, err
	}

	for index, pixels := range texture.levels {
		texture.levels[index] = append([]byte(nil), pixels...)
	}

	return texture, nil
}

// FromOwned admits GPU-ready bytes whose owner relinquishes mutation. It copies
// only the mip-slice descriptors, retaining the immutable pixel storage. Normal
// callers use Create; internal asset loaders already own their received payload.
func FromOwned(data Data) (*Texture, error) {
	if data.ColorSpace != SRGB && data.ColorSpace != Linear {
		return nil, errColorSpace
	}

	if data.Width < 1 || data.Height < 1 || data.Width > maxDimension || data.Height > maxDimension {
		return nil, errDimensions
	}

	if err := validateRepresentation(data); err != nil {
		return nil, err
	}

	if len(data.Levels) == 0 {
		return nil, errMipLevels
	}

	width, height := data.Width, data.Height

	var bytes uint64

	for level, pixels := range data.Levels {
		if level > 0 && width == 0 && height == 0 {
			return nil, errMipLevels
		}

		width, height = max(width, 1), max(height, 1)

		rowBytes, rows := Layout(data.Format, width, height)
		if len(pixels) != rowBytes*rows {
			return nil, errMipLevels
		}

		bytes += uint64(len(pixels))

		if width == 1 && height == 1 {
			width, height = 0, 0
		} else {
			width, height = width>>1, height>>1
		}
	}

	levels := append([][]byte(nil), data.Levels...)

	return &Texture{width: data.Width, height: data.Height, bytes: bytes, levels: levels, space: data.ColorSpace, format: data.Format}, nil
}

// Format identifies the exact GPU upload representation; RGBA8 is the default.
type Format uint8

const (
	RGBA8 Format = iota
	BC1
	ETC2RGB
	BC6H
	BC7
	ETC2RGBA
	ASTC4x4
	ASTC6x6
)

// BlockDimension returns the prepared format's texel block side.
func BlockDimension(format Format) int {
	if format == ASTC6x6 {
		return astcBlockDimension
	}

	return blockDimension
}

// Layout returns tightly packed row bytes and block rows for one mip.
func Layout(format Format, width, height int) (int, int) {
	if format == BC1 || format == ETC2RGB {
		return ((width + blockDimension - 1) / blockDimension) * blockBytes, (height + blockDimension - 1) / blockDimension
	}

	if format == BC6H || format == BC7 || format == ETC2RGBA || format == ASTC4x4 || format == ASTC6x6 {
		side := BlockDimension(format)

		return ((width + side - 1) / side) * wideBlockBytes, (height + side - 1) / side
	}

	return width * Channels, height
}

// CopyExtent returns the physical texel extent covered by a complete mip
// upload. Compressed mip dimensions round up to whole blocks, including mips
// larger than one block whose logical dimensions are not block-aligned.
func CopyExtent(format Format, width, height int) (int, int) {
	if format != RGBA8 {
		side := BlockDimension(format)

		return ((width + side - 1) / side) * side, ((height + side - 1) / side) * side
	}

	return width, height
}

// Encoding identifies the already prepared GPU block representation.
func Encoding(texture *Texture) Format { return texture.format }

// Levels returns immutable mip payloads for internal renderer uploads.
func Levels(texture *Texture) [][]byte { return texture.levels }

// Space returns the texture encoding.
func Space(texture *Texture) ColorSpace { return texture.space }

// Valid reports whether a handle has not been unloaded.
func Valid(texture *Texture) bool { return texture != nil && !texture.released }

// Release drops the retained payload after the App releases GPU caches.
func Release(texture *Texture) { texture.released = true; texture.levels = nil }

// Solid creates a one-pixel fallback texture for the standard shader.
func Solid(pixel [4]byte, space ColorSpace) *Texture {
	return &Texture{width: 1, height: 1, bytes: Channels, levels: [][]byte{append([]byte(nil), pixel[:]...)}, space: space}
}

// CreateStorage allocates a GPU-produced linear RGBA8 texture descriptor, no CPU pixels.
func CreateStorage(width, height int) (*Texture, error) {
	if width < 1 || height < 1 || width > maxDimension || height > maxDimension {
		return nil, errDimensions
	}

	return &Texture{width: width, height: height, space: Linear, storage: true, bytes: uint64(width) * uint64(height) * Channels}, nil
}

// IsStorage identifies mutable GPU output; prepared assets remain immutable.
func IsStorage(image *Texture) bool { return image != nil && image.storage }

// ByteSize is the GPU mip payload size cached during handle creation.
// It includes storage images; it excludes render targets and driver padding.
func ByteSize(texture *Texture) uint64 { return texture.bytes }

func validateRepresentation(data Data) error {
	side := BlockDimension(data.Format)
	if data.Format > ASTC6x6 || data.Format != RGBA8 && (data.Width%side != 0 || data.Height%side != 0) {
		return errMipLevels
	}

	if data.Format == BC6H && data.ColorSpace != Linear {
		return errColorSpace
	}

	return nil
}
