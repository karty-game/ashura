//go:build !js

package impostorbake

import (
	"bytes"
	"slices"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
)

func TestLayeredCoplanarColorUsesLaterPartIndependentOfGatherOrder(t *testing.T) {
	t.Parallel()

	source := closedBoxes([]vector{{0, 0, 0}, {0, 0, 0}})
	source.Parts[0].Material.Color = [4]float32{1, 0, 0, 1}
	source.Parts[1].Material.Color = [4]float32{0, 1, 0, 1}

	geometry, err := prepare(source)
	if err != nil {
		t.Fatal(err)
	}

	options := Options{Mode: impostordata.ModeLayered, Resolution: 32, Padding: 4, MaxIntervals: 2, MipLevels: 1}

	layout, err := impostordata.LayeredLayout(32, 4, 2)
	if err != nil {
		t.Fatal(err)
	}

	pixels, err := layeredRaster(t.Context(), geometry, impostordata.LayeredViews()[2], 32)
	if err != nil {
		t.Fatal(err)
	}

	total := int(layout.Width) * int(layout.Height) * 4
	firstColor, firstNumeric := make([]byte, total), make([]byte, total)

	var report Report
	if err := packLayeredAxis(geometry, pixels, firstColor, firstNumeric, layout, options, 2, &report); err != nil {
		t.Fatal(err)
	}

	slot := layerTileIndex(layout, options, 2*(2*2), 16, 16, 0)
	if !bytes.Equal(firstColor[slot:slot+4], []byte{0, 255, 0, 255}) {
		t.Fatalf("coplanar tie lost later material: %v", firstColor[slot:slot+4])
	}

	for _, hits := range pixels {
		slices.Reverse(hits)
	}

	secondColor, secondNumeric := make([]byte, total), make([]byte, total)
	if err := packLayeredAxis(geometry, pixels, secondColor, secondNumeric, layout, options, 2, &report); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(firstColor, secondColor) || !bytes.Equal(firstNumeric, secondNumeric) {
		t.Fatal("intersection gather order changed coplanar shading")
	}
}
