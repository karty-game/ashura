// Package ktx2 loads prepared RGBA8, BC, ETC2 and ASTC KTX2 files.
// It performs no source-image decoding, decompression or transcoding.
package ktx2

import (
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/pkg/texture"
)

var (
	// ErrFormat marks an unsupported KTX2 profile or a texture role mismatch.
	ErrFormat = ktx2data.ErrFormat
	// ErrBounds marks malformed dimensions, metadata, mip ranges or file size.
	ErrBounds = ktx2data.ErrBounds
)

var errFormat = ErrFormat

// Decode reads a prepared KTX2 container; returned mip slices reference encoded.
// App.NewTexture copies the payloads when taking ownership.
func Decode(encoded []byte) (texture.Data, error) { return ktx2data.Decode(encoded) }
