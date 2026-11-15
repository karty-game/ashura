//go:build !js

package ui

import (
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
)

func TestShapingScratchPreservesWrappedReferenceAndRetainedLayouts(t *testing.T) {
	t.Parallel()

	metrics, err := os.ReadFile("fonts/defaultfont/prepared/glyphs.bin")
	if err != nil {
		t.Fatal(err)
	}

	pixels, err := os.ReadFile("fonts/defaultfont/prepared/atlas.rgba")
	if err != nil {
		t.Fatal(err)
	}

	load := func() *View {
		font, err := LoadFont(goregular.TTF, metrics, pixels)
		if err != nil {
			t.Fatal(err)
		}

		return &View{font: font}
	}
	optimized, reference := load(), load()

	cases := []string{
		"",
		"16.67 ms",
		"-12.4 KiB/s",
		"1234 ",
		"   ",
		"office café",
		"שלום 12",
		"مرحبا 42",
		"123 אבג XYZ",
		"a\nb",
		"a\n\nb",
	}
	for _, text := range cases {
		for _, width := range []float32{12, 73.25, 360} {
			for _, wrap := range []bool{false, true} {
				assertShapingReference(t, optimized, reference, text, width, wrap)
			}
		}
	}

	retained, err := optimized.shape("123.45 ms", 12, 360, true)
	if err != nil {
		t.Fatal(err)
	}

	glyphs, carets := slices.Clone(retained.glyphs), slices.Clone(retained.carets)

	for _, text := range cases {
		if _, err := optimized.shape(text, 24, 180, true); err != nil {
			t.Fatal(err)
		}
	}

	if !slices.Equal(retained.glyphs, glyphs) || !slices.Equal(retained.carets, carets) {
		t.Fatal("shaping scratch overwrote a retained layout")
	}
}

func assertShapingReference(t *testing.T, optimized, reference *View, text string, width float32, wrap bool) {
	t.Helper()

	want := textLayout{text: text, size: 12, width: width}
	if !wrap {
		want.width = 32768
	}

	offset := 0
	for paragraph := range strings.SplitSeq(text, "\n") {
		reference.referenceShapeParagraph(&want, paragraph, offset, 12, want.width, wrap)
		offset += len(paragraph) + 1
	}

	got, err := optimized.shape(text, 12, width, wrap)
	if err != nil {
		t.Fatal(err)
	}

	if got.height != want.height || got.width != want.width || !slices.Equal(got.glyphs, want.glyphs) ||
		!slices.Equal(got.carets, want.carets) {
		t.Fatalf("layout changed for %q width=%g wrap=%t: got=%+v want=%+v", text, width, wrap, got, want)
	}
}

// The previous path delegates mandatory breaks and line fitting to the pinned
// wrapper. Compare its full glyph/caret output with the optimized path.
func (v *View) referenceShapeParagraph(result *textLayout, text string, offset int, size, width float32, wrap bool) {
	faceData := v.font
	runes := []rune(text)

	byteOffsets := make([]int, 0, len(runes)+1)
	for index := range text {
		byteOffsets = append(byteOffsets, index)
	}

	byteOffsets = append(byteOffsets, len(text))
	if len(runes) == 0 {
		result.carets = append(result.carets, caret{offset: offset, positionY: result.height})
		result.height += size * 1.4

		return
	}

	input := shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Size: fixed.Int26_6(size * 64)}
	inputs := faceData.segmenter.Split(input, faceData)

	outputs := make([]shaping.Output, 0, len(inputs))
	for _, run := range inputs {
		outputs = append(outputs, faceData.shaper.Shape(run))
	}

	lineWidth := int(width)
	if !wrap {
		lineWidth = 1 << 24
	}

	lines, _ := faceData.wrapper.WrapParagraph(
		shaping.WrapConfig{Direction: di.DirectionLTR, DisableTrailingWhitespaceTrim: !wrap},
		lineWidth,
		runes,
		shaping.NewSliceIterator(outputs),
	)
	for _, line := range lines {
		sort.SliceStable(line, func(index, j int) bool { return line[index].VisualIndex < line[j].VisualIndex })

		positionX := float32(0)

		for _, run := range line {
			positionX = v.appendRun(result, run, byteOffsets, offset, size, positionX)
		}

		result.height += size * 1.4
	}
}
