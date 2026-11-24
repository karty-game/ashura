//go:build !js

package impostorbake

import (
	"encoding/binary"
	"math"
	"strconv"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/modeldata"
)

func TestLayeredUnionPreservesSolidsAndRejectsInvalidGeometry(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name           string
		hits           []layerHit
		maximum, count int
		valid          bool
	}{
		{"box", []layerHit{{depth: 1, delta: 1}, {depth: -1, delta: -1}}, 1, 1, true},
		{"overlap", []layerHit{{depth: 2, delta: 1}, {depth: 1, delta: 1}, {depth: 0, delta: -1}, {depth: -1, delta: -1}}, 1, 1, true},
		{"separate", []layerHit{{depth: 2, delta: 1}, {depth: 1, delta: -1}, {depth: 0, delta: 1}, {depth: -1, delta: -1}}, 2, 2, true},
		{"coincident", []layerHit{{depth: 1, delta: 1}, {depth: 1, delta: 1}, {depth: -1, delta: -1}, {depth: -1, delta: -1}}, 1, 1, true},
		{"overflow", []layerHit{{depth: 2, delta: 1}, {depth: 1, delta: -1}, {depth: 0, delta: 1}, {depth: -1, delta: -1}}, 1, 0, false},
		{"open", []layerHit{{depth: 1, delta: 1}}, 1, 0, false},
		{"inverted", []layerHit{{depth: 1, delta: -1}, {depth: -1, delta: 1}}, 1, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var report Report

			intervals, err := unionLayers(test.hits, 3, test.maximum, &report)
			if (err == nil) != test.valid {
				t.Fatalf("intervals%v error%v", intervals, err)
			}

			if test.valid && len(intervals) != test.count {
				t.Fatalf("union intervals%d expected%d", len(intervals), test.count)
			}
		})
	}
}

func TestLayeredNegativeExcursionsRequireTinyClosedPocket(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		depth float64
		valid bool
	}{{5e-5, true}, {2e-4, false}} {
		var report Report

		intervals, err := unionLayers([]layerHit{{depth: .5, delta: -1}, {depth: .5 - test.depth, delta: 1}}, 1, 8, &report)
		if (err == nil) != test.valid {
			t.Fatalf("excursion%g intervals%v error%v", test.depth, intervals, err)
		}

		if test.valid && (report.ToleratedNegativePockets != 1 || math.Abs(report.MaximumNegativePocketDepth-test.depth) > 1e-12) {
			t.Fatalf("numeric pocket was not disclosed: %+v", report)
		}
	}
}

func TestLayeredBakesPairsOutwardNormalsAndConservativeHierarchy(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		centers   []vector
		intervals int
	}{
		{"box", []vector{{0, 0, 0}}, 1},
		{"overlap", []vector{{0, 0, -.5}, {0, 0, .5}}, 1},
		{"separate", []vector{{0, 0, -2}, {0, 0, 2}}, 2},
		{"coincident", []vector{{0, 0, 0}, {0, 0, 0}}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := closedBoxes(test.centers)
			options := Options{Mode: impostordata.ModeLayered, Resolution: 32, MaxIntervals: 4}

			baked, err := Bake(t.Context(), source, options)
			if err != nil {
				t.Fatal(err)
			}

			numeric := layerPixels(t, baked.NormalDepth, modeldata.Occlusion)
			color := layerPixels(t, baked.Color, modeldata.Albedo)

			layout, err := impostordata.LayeredLayout(32, 4, 4)
			if err != nil {
				t.Fatal(err)
			}

			target := hierarchyIndex(layout, 2, (32*32-1)/3+16*32+16)
			if int(numeric[target+2]) != test.intervals {
				t.Fatalf("center intervals%d expected%d", numeric[target+2], test.intervals)
			}

			pair := layerTileIndex(layout, Options{Resolution: 32, Padding: 4}, 2*4, 16, 16, int(layout.PairBaseRow))

			front, back := binary.LittleEndian.Uint16(numeric[pair:]), binary.LittleEndian.Uint16(numeric[pair+2:])
			if front < back || front == 0 || back == 0 {
				t.Fatal("invalid interval endpoints")
			}

			for side, expected := range []vector{{0, 0, 1}, {0, 0, -1}} {
				slot := layerTileIndex(layout, Options{Resolution: 32, Padding: 4}, 2*8+side, 16, 16, 0)

				normal := decodeOct(numeric[slot], numeric[slot+1])
				if dot(normal, expected) < .999 || color[slot+3] != 255 {
					t.Fatalf("outward boundary normal%v expected%v", normal, expected)
				}
			}

			checkHierarchy(t, numeric, layout, 32)

			encoded, err := impostordata.Encode(baked)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := impostordata.Decode(encoded); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLayeredSettingsRejectOversizedIntegersBeforeNarrowing(t *testing.T) {
	t.Parallel()

	settings := []Options{
		{Mode: impostordata.ModeLayered, MaxIntervals: int(^uint(0) >> 1)},
		{Mode: impostordata.ModeLayered, Padding: int(^uint(0) >> 1)},
		{Mode: impostordata.ModeLayered, MaxIntervals: 17},
		{Mode: impostordata.ModeLayered, MipLevels: 2},
		{Mode: impostordata.ModeLayered, Resolution: 80},
		{Mode: impostordata.ModeOctahedral, MaxIntervals: 8},
	}

	if strconv.IntSize == 64 {
		huge := int64(1) << 32
		settings = append(
			settings,
			Options{Mode: impostordata.ModeLayered, MaxIntervals: int(huge + 8)},
			Options{Mode: impostordata.ModeLayered, Padding: int(huge + 4)},
		)
	}

	for _, options := range settings {
		if _, err := Defaults(options); err == nil {
			t.Fatalf("accepted incompatible settings %+v", options)
		}
	}
}

func layerPixels(t *testing.T, data []byte, role modeldata.TextureRole) []byte {
	t.Helper()

	value, err := decodeTexture(modeldata.Texture{Role: role, Data: data})
	if err != nil {
		t.Fatal(err)
	}

	return value.data
}
func decodeOct(r, g byte) vector {
	horizontal, vertical := float64(r)/255*2-1, float64(g)/255*2-1

	z := 1 - math.Abs(horizontal) - math.Abs(vertical)
	if z < 0 {
		horizontal, vertical = (1-math.Abs(vertical))*signNonZero(horizontal), (1-math.Abs(horizontal))*signNonZero(vertical)
	}

	return unit(vector{horizontal, vertical, z})
}
func checkHierarchy(t *testing.T, numeric []byte, layout impostordata.LayerLayout, resolution int) {
	t.Helper()

	for axis := range 3 {
		for side := 1; side < resolution; side *= 2 {
			for row := range side {
				for column := range side {
					parent := hierarchyIndex(layout, axis, (side*side-1)/3+row*side+column)

					for dy := range 2 {
						for dx := range 2 {
							child := hierarchyIndex(layout, axis, (4*side*side-1)/3+(2*row+dy)*2*side+2*column+dx)
							if numeric[child+3] != 0 &&
								(numeric[parent+3] == 0 || numeric[parent] > numeric[child] || numeric[parent+1] < numeric[child+1] || numeric[parent+2] < numeric[child+2]) {
								t.Fatal("parent is not conservative")
							}
						}
					}
				}
			}
		}
	}
}

//nolint:gosec // Fixed box fixtures append only 24 vertices per independently bounded part.
func closedBoxes(centers []vector) modeldata.Document {
	document := modeldata.Document{Source: "closed-boxes"}

	for _, center := range centers {
		part := modeldata.Part{
			Material: modeldata.Material{Color: [4]float32{.3, .6, .2, 1}, Roughness: .7, Albedo: -1, Normal: -1, AO: -1},
		}

		for axis := range 3 {
			for _, sign := range []float64{-1, 1} {
				base := uint32(len(part.Vertices) / 32)
				other, next := (axis+1)%3, (axis+2)%3

				for _, coordinates := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
					position := center
					position[axis] += sign
					position[other] += coordinates[0]
					position[next] += coordinates[1]
					normal := vector{}

					normal[axis] = sign
					for _, value := range []float32{float32(position[0]), float32(position[1]), float32(position[2]), float32(normal[0]), float32(normal[1]), float32(normal[2]), float32((coordinates[0] + 1) / 2), float32((coordinates[1] + 1) / 2)} {
						part.Vertices = binary.LittleEndian.AppendUint32(part.Vertices, math.Float32bits(value))
					}
				}

				order := []uint32{0, 1, 2, 0, 2, 3}
				if sign < 0 {
					order = []uint32{0, 2, 1, 0, 3, 2}
				}

				for _, index := range order {
					part.Indices = binary.LittleEndian.AppendUint32(part.Indices, base+index)
				}
			}
		}

		document.Parts = append(document.Parts, part)
	}

	return document
}

func TestLayeredRejectsFragmentWorkingSetOverflow(t *testing.T) {
	t.Parallel()

	fragments := layerFragments{pixels: make([][]layerHit, 16), limit: 1}
	points := [3]projected{{column: 0, row: 0}, {column: 4, row: 0}, {column: 0, row: 4}}

	err := rasterLayerTriangle(&fragments, points, [3]int{0, 1, 2}, 16, 0, 1, 4)
	if err == nil || fragments.count != 1 {
		t.Fatalf("unbounded all-intersection raster: count%d error%v", fragments.count, err)
	}
}
