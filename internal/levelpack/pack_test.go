//go:build !js

package levelpack

import (
	"context"
	"encoding/binary"
	"image"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/texturedata"
)

func fixtureInput(offset float32) Input {
	material := modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Roughness: 1, Albedo: -1, Normal: -1, AO: -1}
	part := modeldata.Part{Name: "wall", Material: material}

	var coordinates []byte

	for index, vertex := range [][3]float32{{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0}} {
		for _, value := range [8]float32{vertex[0] + offset, vertex[1], vertex[2], 0, 0, 1, vertex[0] * 5, vertex[1] * 7} {
			part.Vertices = binary.LittleEndian.AppendUint32(part.Vertices, math.Float32bits(value))
		}

		for _, value := range [][2]float32{{.25, .25}, {.5, .25}, {.5, .5}, {.25, .5}}[index] {
			coordinates = binary.LittleEndian.AppendUint32(coordinates, math.Float32bits(value))
		}
	}

	for _, index := range []uint32{0, 1, 2, 0, 2, 3} {
		part.Indices = binary.LittleEndian.AppendUint32(part.Indices, index)
	}

	input := Input{Model: modeldata.Document{Source: "fixture", Parts: []modeldata.Part{part}}, UVs: [][]byte{coordinates},
		Materials: []leveldata.Material{{Name: "wall", Color: material.Color}}, Scale: 3, Encoding: "rgbe8"}
	for index := range input.Maps {
		pixels := make([]byte, 64*64*4)

		for y := range 64 {
			for x := range 64 {
				copy(pixels[(y*64+x)*4:], []byte{byte(x), byte(y), byte(index) + byte(offset)*10, 125})
			}
		}

		input.Maps[index] = texturedata.Data{Width: 64, Height: 64, ColorSpace: texturedata.Linear, Levels: [][]byte{pixels}}
	}

	return input
}

func TestConsolidationPreservesSurfaceAndLighting(t *testing.T) {
	t.Parallel()

	inputs := []Input{fixtureInput(0), fixtureInput(2)}

	output, err := Pack(t.Context(), inputs, 2048)
	if err != nil || len(output) != 1 || len(output[0].Model.Parts) != 1 {
		t.Fatalf("compatible source ranges were not merged: pages=%d err=%v", len(output), err)
	}

	atlas := output[0]
	if atlas.Scale != 3 || atlas.Encoding != "rgbe8" || atlas.Size > 64 {
		t.Fatalf("changed encoding or retained oversized sparse atlas: %+v", atlas)
	}

	part := atlas.Model.Parts[0]
	if len(part.Indices) != 48 || len(part.Vertices) != 8*32 {
		t.Fatal("geometry lost or duplicated")
	}

	for offset := 0; offset < len(part.Vertices); offset += 32 {
		positionX := math.Float32frombits(binary.LittleEndian.Uint32(part.Vertices[offset:]))

		sourceIndex := 0
		if positionX >= 2 {
			sourceIndex = 1
		}

		source := inputs[sourceIndex]
		original := -1

		for vertex := range 4 {
			if reflect.DeepEqual(part.Vertices[offset:offset+32], source.Model.Parts[0].Vertices[vertex*32:(vertex+1)*32]) {
				original = vertex
			}
		}

		if original < 0 {
			t.Fatal("position, normal or repeating UV0 changed")
		}
		// Check the complete bilinear support around every vertex, including gutters.
		for mapIndex := range 3 {
			for _, delta := range [][2]int{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}, {1, 1}} {
				actual := sampledPixel(atlas.Maps[mapIndex], atlas.UVs[0][offset/4:], atlas.Size, delta)

				expected := sampledPixel(source.Maps[mapIndex].Levels[0], source.UVs[0][original*8:], 64, delta)
				if actual != expected {
					t.Fatalf("lighting/gutter resampled: got %v want %v", actual, expected)
				}
			}
		}
	}

	again, err := Pack(t.Context(), inputs, 2048)
	if err != nil || !reflect.DeepEqual(output, again) {
		t.Fatal("nondeterministic output", err)
	}
}

func sampledPixel(pixels, coordinates []byte, size int, delta [2]int) [4]byte {
	x := int(math.Float32frombits(binary.LittleEndian.Uint32(coordinates))*float32(size)) + delta[0]
	y := int(math.Float32frombits(binary.LittleEndian.Uint32(coordinates[4:]))*float32(size)) + delta[1]

	return [4]byte(pixels[(y*size+x)*4 : (y*size+x)*4+4])
}

func TestConsolidationRetainsIncompatibleFamiliesAndBounds(t *testing.T) {
	t.Parallel()

	inputs := []Input{fixtureInput(0), fixtureInput(2)}
	inputs[1].Scale = 4

	output, err := Pack(t.Context(), inputs, 2048)
	if err != nil || len(output) != 2 {
		t.Fatal("merged incompatible numeric scales", err)
	}

	inputs[1].Scale = inputs[0].Scale
	inputs[1].Materials[0].DoubleSided = true

	output, err = Pack(t.Context(), inputs, 2048)
	if err != nil || len(output) != 1 || len(output[0].Materials) != 2 {
		t.Fatal("merged incompatible materials", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Pack(ctx, inputs, 2048); err == nil {
		t.Fatal("ignored cancellation")
	}

	inputs[0].UVs[0] = []byte{0}
	if _, err := Pack(t.Context(), inputs, 2048); err == nil {
		t.Fatal("accepted mismatched UV stream")
	}
}

// referenceInsert retains the original all-pairs MaxRects pruning. Comparing
// each insertion checks placement, stable ties and the complete free-space set,
// rather than merely checking that final rectangles do not overlap.
func referenceInsert(value *atlas, component chart) bool {
	best, score := -1, int(^uint(0)>>1)

	width, height := component.bounds.Dx(), component.bounds.Dy()
	for index, free := range value.free {
		if width <= free.Dx() && height <= free.Dy() {
			candidate := min(free.Dx()-width, free.Dy()-height)
			if candidate < score {
				best, score = index, candidate
			}
		}
	}

	if best < 0 {
		return false
	}

	component.position = value.free[best].Min
	used := image.Rectangle{Min: component.position, Max: component.position.Add(image.Pt(width, height))}

	var next []image.Rectangle

	for _, free := range value.free {
		if !free.Overlaps(used) {
			next = append(next, free)

			continue
		}

		for _, rectangle := range []image.Rectangle{
			image.Rect(free.Min.X, free.Min.Y, used.Min.X, free.Max.Y),
			image.Rect(used.Max.X, free.Min.Y, free.Max.X, free.Max.Y),
			image.Rect(free.Min.X, free.Min.Y, free.Max.X, used.Min.Y),
			image.Rect(free.Min.X, used.Max.Y, free.Max.X, free.Max.Y),
		} {
			if !rectangle.Empty() && rectangle.In(free) {
				next = append(next, rectangle)
			}
		}
	}

	value.free = nil
	for index, free := range next {
		contained := false

		for other, candidate := range next {
			if other != index && free.In(candidate) && (free != candidate || other < index) {
				contained = true

				break
			}
		}

		if !contained {
			value.free = append(value.free, free)
		}
	}

	value.charts = append(value.charts, component)

	return true
}

func TestIncrementalPruningMatchesAllPairs(t *testing.T) {
	t.Parallel()

	for seed := uint64(1); seed <= 8; seed++ {
		random := rand.New(rand.NewPCG(seed, 17)) //nolint:gosec // Reproducible packing fixture; no security randomness.
		actual := atlas{size: 512, free: []image.Rectangle{image.Rect(0, 0, 512, 512)}}

		reference := atlas{size: 512, free: []image.Rectangle{image.Rect(0, 0, 512, 512)}}
		for index := range 1600 {
			component := chart{bounds: image.Rect(0, 0, 1+random.IntN(48), 1+random.IntN(48))}

			got, want := actual.insert(component), referenceInsert(&reference, component)
			if got != want || !reflect.DeepEqual(actual.free, reference.free) || !reflect.DeepEqual(actual.charts, reference.charts) {
				t.Fatal("MaxRects placement or free-space order changed", seed, index)
			}
		}
	}
}
