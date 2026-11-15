//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

func (v *View) shape(text string, size, width float32, wrap bool) (textLayout, error) {
	faceData := v.font

	if len(text) > MaxTextBytes || size <= 0 || size > 256 || width <= 0 {
		return textLayout{}, ErrBounds
	}

	// Unwrapped shaping does not depend on the control's current width.
	if !wrap {
		width = 32768
	}

	key := textKey{text, size, width, wrap}
	if result, ok := faceData.cachedLayout(key); ok {
		return result, nil
	}

	if !utf8.ValidString(text) {
		return textLayout{}, ErrBounds
	}

	v.stats.Shapes++
	result := textLayout{text: text, size: size, width: width}

	offset := 0
	for paragraph := range strings.SplitSeq(text, "\n") {
		v.shapeParagraph(&result, paragraph, offset, size, width, wrap)

		offset += len(paragraph) + 1
	}

	faceData.cacheLayout(key, result)

	return result, nil
}
func (v *View) shapeParagraph(result *textLayout, text string, offset int, size, width float32, wrap bool) {
	faceData := v.font
	runes := faceData.shapeRunes[:0]
	byteOffsets := faceData.shapeByteOffsets[:0]

	for index, character := range text {
		runes = append(runes, character)
		byteOffsets = append(byteOffsets, index)
	}

	byteOffsets = append(byteOffsets, len(text))

	faceData.shapeRunes, faceData.shapeByteOffsets = runes, byteOffsets
	if len(runes) == 0 {
		result.carets = append(result.carets, caret{offset: offset, positionY: result.height})
		result.height += size * 1.4

		return
	}

	input := shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: fixed.Int26_6(size * 64)}
	inputs := faceData.segmenter.Split(input, faceData)

	outputs := faceData.shapeOutputs[:0]
	for _, run := range inputs {
		outputs = append(outputs, faceData.shaper.Shape(run))
	}

	faceData.shapeOutputs = outputs[:0]
	defer clear(outputs)

	// Layouts in the LRU and on nodes own these arrays. Only shaping scratch
	// storage is reused; later text changes must not overwrite retained layouts.
	result.glyphs = slices.Grow(result.glyphs, len(runes))
	result.carets = slices.Grow(result.carets, 2*len(runes))

	lineWidth := int(width)
	if !wrap {
		lineWidth = 1 << 24
	}
	// The wrapper returns this same single run when it fits and has no mandatory
	// break. Avoid constructing its line/grapheme iterators for ordinary labels.
	if len(outputs) == 1 && outputs[0].Advance <= fixed.I(lineWidth) && !strings.ContainsAny(text, "\r\v\f\u0085\u2028\u2029") {
		v.appendRun(result, outputs[0], byteOffsets, offset, size, 0)
		result.height += size * 1.4

		return
	}

	lines, _ := faceData.wrapper.WrapParagraph(
		shaping.WrapConfig{Direction: di.DirectionLTR, DisableTrailingWhitespaceTrim: !wrap},
		lineWidth,
		runes,
		shaping.NewSliceIterator(outputs),
	)
	for _, line := range lines {
		slices.SortStableFunc(line, func(a, b shaping.Output) int { return cmp.Compare(a.VisualIndex, b.VisualIndex) })

		positionX := float32(0)

		for _, run := range line {
			positionX = v.appendRun(result, run, byteOffsets, offset, size, positionX)
		}

		result.height += size * 1.4
	}
}
func (layoutData textLayout) position(offset int) (float32, float32) {
	positionX, positionY := float32(0), float32(0)

	for _, child := range layoutData.carets {
		if child.offset == offset {
			return child.positionX, child.positionY
		}

		if child.offset < offset {
			positionX, positionY = child.positionX, child.positionY
		}
	}

	return positionX, positionY
}
func (v *View) placeCaret(node *Node, positionX float32, extend bool) {
	if !v.refreshEditorLayout(node) {
		return
	}

	if len(node.layout.carets) == 0 {
		return
	}

	local := positionX - node.rect.X - 12 + node.editOffset
	best, distance := 0, float32(1e20)

	for _, child := range node.layout.carets {
		delta := child.positionX - local
		if delta < 0 {
			delta = -delta
		}

		if delta < distance {
			best, distance = child.offset, delta
		}
	}

	node.cursor = best
	if !extend {
		node.anchor = best
	}

	node.damage(false)

	v.caretVisible = true
}

func (v *View) appendRun(result *textLayout, run shaping.Output, byteOffsets []int, offset int, size, positionX float32) float32 {
	ascent := float32(run.LineBounds.Ascent) / 64
	for index := 0; index < len(run.Glyphs); {
		glyph := run.Glyphs[index]
		startX := positionX

		next := index
		for next < len(run.Glyphs) && run.Glyphs[next].TextIndex() == glyph.TextIndex() {
			glyph := run.Glyphs[next]

			id := int(glyph.GlyphID)
			if id >= len(v.font.glyphs) {
				id = 0
			}

			metric := v.font.glyphs[id]
			scale := size / v.font.size

			if metric.rect.W > 0 {
				record := metric.rect
				record.X = positionX + record.X*scale + float32(glyph.XOffset)/64
				record.Y = result.height + ascent + record.Y*scale - float32(glyph.YOffset)/64
				record.W *= scale
				record.H *= scale
				result.glyphs = append(result.glyphs, positionedGlyph{record, metric.uv})
			}

			positionX += float32(glyph.Advance) / 64
			next++
		}

		start, end := glyph.TextIndex(), min(len(byteOffsets)-1, glyph.TextIndex()+glyph.RunesCount())
		for record := start; record <= end && record < len(byteOffsets); record++ {
			fraction := float32(record-start) / float32(max(1, end-start))
			if run.Direction.Progression() == di.TowardTopLeft {
				fraction = 1 - fraction
			}

			result.carets = append(
				result.carets,
				caret{offset: offset + byteOffsets[record], positionX: startX + (positionX-startX)*fraction, positionY: result.height},
			)
		}

		index = next
	}

	return positionX
}

func (v *View) refreshEditorLayout(node *Node) bool {
	if node.layout.text == node.text && node.layout.size == v.fontSize(node) {
		return true
	}

	layout, err := v.shape(node.text, v.fontSize(node), 32768, false)
	if err != nil {
		v.pending.Err = err

		return false
	}

	node.layout = layout

	return true
}
