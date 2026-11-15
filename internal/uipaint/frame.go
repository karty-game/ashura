// Package uipaint holds the renderer-independent retained UI submission format.
package uipaint

import (
	"encoding/binary"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

const (
	Stride             = 96
	maxPageSide        = 4096
	pageBlockDimension = 4
)

// Texture is an immutable GPU-ready straight-alpha page. Zero Format is RGBA8.
type Texture struct {
	Width, Height int
	Format        texturedata.Format
	Pixels        []byte
}

// ValidPage bounds a single prepared page without inspecting or transforming its payload.
func ValidPage(width, height int, format texturedata.Format, size int) bool {
	if width < 1 || height < 1 || width > maxPageSide || height > maxPageSide {
		return false
	}

	switch format {
	case texturedata.RGBA8:
	case texturedata.BC1, texturedata.BC7, texturedata.ETC2RGB, texturedata.ETC2RGBA, texturedata.ASTC4x4:
		if width%pageBlockDimension != 0 || height%pageBlockDimension != 0 {
			return false
		}
	case texturedata.BC6H:
		return false
	default:
		return false
	}

	rowBytes, rows := texturedata.Layout(format, width, height)

	return size == rowBytes*rows
}

// Batch preserves painter order and references persistent instance slots.
type Batch struct {
	First, Count int
	Texture      *Texture
}

type Range struct{ Offset, Size int }

// Frame is borrowed until the next UI frame; Revision/Base protect skipped frames.
type Frame struct {
	Revision, Base uint64
	Width, Height  float32
	Data           []byte
	Batches        []Batch
	Changes        []Range
}

// Viewport packs the sixteen-byte shared uniform. sRGB attachments need linear output.
func Viewport(width, height float32, linear bool) []byte {
	var values [4]float32

	values[0], values[1] = width, height
	if linear {
		values[2] = 1
	}

	data := make([]byte, 16)
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], bits(v))
	}

	return data
}
