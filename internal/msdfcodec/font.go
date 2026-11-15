//go:build !js

//nolint:mnd // Fixed font profile and pinned wasm32 ABI; requests are bounded before narrowing.
package msdfcodec

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	atlasSize = 1024
	fontSize  = 32
	padding   = 4
)

// Pair owns a complete atlas and matching AFNT0001 metrics. Publication remains
// the asset pipeline's responsibility; preparation never writes project files.
type Pair struct{ Atlas, Metrics []byte }

type glyphRecord struct {
	left, top, width, height, atlasX, atlasY int
}

// PrepareGoRegular stages every glyph, runs the embedded msdfgen C API once per
// nonempty glyph, then packs the unchanged fixed atlas/metrics layout in Go.
func PrepareGoRegular(ctx context.Context) (Pair, error) {
	if ctx == nil {
		return Pair{}, errInvalid
	}

	if err := ctx.Err(); err != nil {
		return Pair{}, err
	}

	face, err := font.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		return Pair{}, err
	}

	count, err := glyphCount(goregular.TTF)
	if err != nil {
		return Pair{}, err
	}

	records, requests, err := stageGlyphs(ctx, face, count)
	if err != nil {
		return Pair{}, err
	}

	pixels, err := rasterize(ctx, goregular.TTF, requests)
	if err != nil {
		return Pair{}, err
	}

	pair, err := encodePair(records, pixels)

	if failure := ctx.Err(); failure != nil {
		return Pair{}, failure
	}

	return pair, err
}

func stageGlyphs(ctx context.Context, face *font.Face, count int) ([]glyphRecord, []glyphRequest, error) {
	records := make([]glyphRecord, count)
	requests := make([]glyphRequest, count)
	scale := float64(fontSize) / float64(face.Upem())
	atlasX, atlasY, rowHeight := 1, 1, 0

	for glyphID := range count {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		outline, exists := face.GlyphDataOutline(font.GID(glyphID))
		if !exists || len(outline.Segments) == 0 {
			continue
		}

		left, bottom, right, top := outlineBounds(outline.Segments, scale)

		width, height := right-left, top-bottom
		if width <= 0 || height <= 0 || width > 512 || height > 512 {
			return nil, nil, errInvalid
		}

		if atlasX+width+1 > atlasSize {
			atlasX = 1
			atlasY += rowHeight + 1
			rowHeight = 0
		}

		if atlasY+height+1 > atlasSize {
			return nil, nil, errInvalid
		}

		records[glyphID] = glyphRecord{left, -top, width, height, atlasX, atlasY}
		requests[glyphID] = glyphRequest{uint32(glyphID), uint32(width), uint32(height), left, bottom}
		atlasX += width + 1
		rowHeight = max(rowHeight, height)
	}

	return records, requests, nil
}

func outlineBounds(segments []opentype.Segment, scale float64) (int, int, int, int) {
	left, bottom, right, top := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)

	for _, segment := range segments {
		for _, point := range segment.ArgsSlice() {
			x, y := float64(point.X)*scale, float64(point.Y)*scale
			left, bottom, right, top = min(left, x), min(bottom, y), max(right, x), max(top, y)
		}
	}

	return int(math.Floor(left)) - padding, int(math.Floor(bottom)) - padding,
		int(math.Ceil(right)) + padding, int(math.Ceil(top)) + padding
}

func glyphCount(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, errInvalid
	}

	count := int(binary.BigEndian.Uint16(data[4:]))
	if count > (len(data)-12)/16 {
		return 0, errInvalid
	}

	for index := range count {
		entry := data[12+index*16:]
		if string(entry[:4]) != "maxp" {
			continue
		}

		offset := uint64(binary.BigEndian.Uint32(entry[8:]))
		if offset+6 > uint64(len(data)) {
			return 0, errInvalid
		}

		return int(binary.BigEndian.Uint16(data[offset+4:])), nil
	}

	return 0, errInvalid
}
