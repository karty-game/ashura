// Package spritepaint defines the shared retained 2D submission ABI.
package spritepaint

import "github.com/karty-game/ashura/internal/uipaint"

// Stride packs two transform vectors, UV bounds, linear tint/opacity and a clip.
const Stride = 80

// Texture reuses the immutable prepared RGBA page representation, without UI controls.
type Texture = uipaint.Texture

// Range identifies changed bytes in the persistent instance record buffer.
type Range struct{ Offset, Size int }

// Batch preserves painter order across contiguous persistent instance slots.
type Batch struct {
	First, Count int
	Texture      *Texture
	Clip         [4]uint32
}

// Frame is borrowed until the next screen frame. Base identifies the buffer
// revision to which Changes apply; consumers of skipped frames upload Data.
type Frame struct {
	Revision, Base uint64
	// BatchesRevision changes only when ordering, page or scissor spans change.
	BatchesRevision uint64
	Width, Height   float32
	Data            []byte
	Batches         []Batch
	// Textures includes pages referenced by live sprites, including hidden sprites.
	Textures []*Texture
	Changes  []Range
}
