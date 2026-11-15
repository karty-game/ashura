//go:build !js

package impostorbake

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
)

func TestOctahedralDirectionsCoverBothHemispheresAndPoles(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		column, row int
		outward     vector
	}{
		{2, 2, vector{0, 1, 0}}, {0, 0, vector{0, -1, 0}},
		{4, 2, vector{1, 0, 0}}, {0, 2, vector{-1, 0, 0}},
		{2, 4, vector{0, 0, 1}}, {2, 0, vector{0, 0, -1}},
	} {
		view := octahedralView(test.column, test.row, 5)
		if doubleVector(view.Outward) != test.outward {
			t.Fatalf("capture(%d,%d) direction %v", test.column, test.row, view.Outward)
		}
	}

	for grid := 4; grid <= 8; grid++ {
		for row := range grid {
			for column := range grid {
				view := octahedralView(column, row, grid)

				right, up, outward := doubleVector(view.Right), doubleVector(view.Up), doubleVector(view.Outward)
				if math.Abs(dot(outward, outward)-1) > 1e-6 || dot(cross(right, up), outward) < .999999 {
					t.Fatalf("nonfinite or reflected capture basis: %+v", view)
				}
			}
		}
	}
}

func TestOctahedralDefaultProducesBoundedPreparedFullSphere(t *testing.T) {
	t.Parallel()

	baked, err := Bake(t.Context(), fixtureDocument(), Options{Mode: impostordata.ModeOctahedral})
	if err != nil {
		t.Fatal(err)
	}

	if baked.Mode != impostordata.ModeOctahedral || baked.Resolution != 80 || baked.Columns != 8 ||
		baked.Azimuths != 8 || baked.Elevations != 8 || len(baked.Views) != 64 || baked.AtlasWidth != 704 || baked.AtlasHeight != 704 {
		t.Fatalf("incorrect full-sphere capture layout: %+v", baked)
	}

	if len(baked.Vertices) != 4*meshdata.VertexStride || len(baked.Indices) != 6*meshdata.IndexStride {
		t.Fatal("full-sphere captures changed the prepared quad budget")
	}

	if got := textureTexelBytes(baked.Color) + textureTexelBytes(baked.NormalDepth); got != 5203968 {
		t.Fatalf("capture texture budget changed: %d", got)
	}

	encoded, err := impostordata.Encode(baked)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := impostordata.Decode(encoded)
	if err != nil || len(decoded.Uniform) != impostordata.UniformSize {
		t.Fatalf("prepared shader table mismatch: %v", err)
	}
}

func TestOctahedralResolutionRequiresMipAlignment(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		resolution, grid int
		valid            bool
	}{{80, 8, true}, {84, 4, true}, {82, 8, false}, {80, 3, false}, {80, 9, false}} {
		_, err := Defaults(Options{Mode: impostordata.ModeOctahedral, Resolution: test.resolution, FrameGrid: test.grid})
		if (err == nil) != test.valid {
			t.Fatalf("resolution%d grid%d: %v", test.resolution, test.grid, err)
		}
	}
}

func textureTexelBytes(data []byte) int {
	levels := binary.LittleEndian.Uint32(data[40:])

	var total uint64
	for level := range levels {
		total += binary.LittleEndian.Uint64(data[80+level*24+8:])
	}

	return int(total)
}
