package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type glyphMetric struct{ rect, uv Rect }

// Font owns a prepared MSDF atlas and parsed shaping tables. It never rasterizes.
type Font struct {
	face                 *font.Face
	page                 *uipaint.Texture
	size, pxRange        float32
	glyphs               []glyphMetric
	shaper               shaping.HarfbuzzShaper
	segmenter            shaping.Segmenter
	wrapper              shaping.LineWrapper
	shapeRunes           []rune
	shapeByteOffsets     []int
	shapeOutputs         []shaping.Output
	cache                map[textKey]int
	cacheEntries         []textCacheEntry
	cacheHead, cacheTail int
}
type textKey struct {
	text        string
	size, width float32
	wrap        bool
}
type positionedGlyph struct{ rect, uv Rect }
type caret struct {
	offset               int
	positionX, positionY float32
}
type textLayout struct {
	text                string
	size, width, height float32
	glyphs              []positionedGlyph
	carets              []caret
}

// LoadFont admits exact offline AFNT0001 metrics and matching GPU-ready RGBA bytes.
func LoadFont(ttf, metrics, pixels []byte) (*Font, error) {
	return LoadPreparedFont(ttf, metrics, pixels, texture.RGBA8)
}

// LoadPreparedFont admits AFNT0001 metrics and a matching single GPU-ready page.
// Metrics must hash the actual prepared payload. Numeric distance channels are
// sampled linearly; compressed formats require enabled device support.
func LoadPreparedFont(ttf, metrics, pixels []byte, format texture.Format) (*Font, error) {
	if len(ttf) == 0 || len(ttf) > 32<<20 || len(metrics) < 92 || string(metrics[:8]) != "AFNT0001" {
		return nil, ErrBounds
	}

	width, height := int(binary.LittleEndian.Uint32(metrics[8:])), int(binary.LittleEndian.Uint32(metrics[12:]))
	size, pxRange := binary.LittleEndian.Uint32(metrics[16:]), binary.LittleEndian.Uint32(metrics[20:])

	count := int(binary.LittleEndian.Uint32(metrics[24:]))
	if !validFontDimensions(width, height, size, pxRange, count) || len(metrics) != 92+36*count ||
		!uipaint.ValidPage(width, height, format, len(pixels)) {
		return nil, ErrBounds
	}

	fontHash, pixelHash := sha256.Sum256(ttf), sha256.Sum256(pixels)
	if !bytes.Equal(metrics[28:60], fontHash[:]) || !bytes.Equal(metrics[60:92], pixelHash[:]) {
		return nil, ErrBounds
	}

	face, err := font.ParseTTF(bytes.NewReader(ttf))
	if err != nil {
		return nil, err
	}

	faceData := &Font{
		face:         face,
		page:         &uipaint.Texture{Width: width, Height: height, Format: format, Pixels: append([]byte(nil), pixels...)},
		size:         float32(size),
		pxRange:      float32(pxRange),
		glyphs:       make([]glyphMetric, count),
		cache:        make(map[textKey]int),
		cacheEntries: make([]textCacheEntry, 1, maxTextLayouts+1),
	}
	for index := range count {
		metric, err := readGlyphMetric(metrics[92+index*36:], index)
		if err != nil {
			return nil, err
		}

		faceData.glyphs[index] = metric
	}

	return faceData, nil
}

func (faceData *Font) ResolveFace(rune) *font.Face { return faceData.face }

func readGlyphMetric(record []byte, index int) (glyphMetric, error) {
	//nolint:gosec // The caller bounds this index to the validated maximum of 65,535 glyphs.
	if binary.LittleEndian.Uint32(record) != uint32(index) {
		return glyphMetric{}, ErrBounds
	}

	var values [8]float32
	for j := range values {
		values[j] = math.Float32frombits(binary.LittleEndian.Uint32(record[4+j*4:]))
		if !finite(values[j]) {
			return glyphMetric{}, ErrBounds
		}
	}

	if values[2] < 0 || values[3] < 0 || values[2] > 512 || values[3] > 512 || values[4] < 0 || values[5] < 0 || values[6] < 0 ||
		values[7] < 0 ||
		values[4]+values[6] > 1 ||
		values[5]+values[7] > 1 {
		return glyphMetric{}, ErrBounds
	}

	return glyphMetric{
		rect: Rect{X: values[0], Y: values[1], W: values[2], H: values[3]},
		uv:   Rect{X: values[4], Y: values[5], W: values[6], H: values[7]},
	}, nil
}

func validFontDimensions(width, height int, size, pxRange uint32, count int) bool {
	return width > 0 && height > 0 && width <= 4096 && height <= 4096 && size > 0 && size <= 256 && pxRange > 0 && pxRange <= 32 &&
		count > 0 &&
		count <= 65535
}
