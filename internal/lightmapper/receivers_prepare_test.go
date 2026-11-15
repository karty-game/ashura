package lightmapper

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
)

// Freeze the original serial accumulation traversal to guard exact geometry,
// overlap admission and floating-point order against row-domain parallelism.
func serialReceiverReference(triangles []Triangle, uv1 [][3]Vec2, width, height uint32) ([]Receiver, error) {
	byPixel := make(map[uint32]*Receiver)

	for index, triangleUV := range uv1 {
		pieces, err := receiverClipTriangle(uint32(index), triangleUV, width, height)
		if err != nil {
			return nil, fmt.Errorf("triangle %d (id %d), UV1 %v: %w", index, triangles[index].ID, triangleUV, err)
		}

		for _, piece := range pieces {
			receiver := byPixel[piece.pixel]
			if receiver == nil {
				receiver = &Receiver{ID: uint64(piece.pixel) + 1, Pixel: piece.pixel, Side: 1}
				byPixel[piece.pixel] = receiver
			}

			for _, existing := range receiver.Pieces {
				if receiverPositiveUVOverlap(piece.polygon, uv1[existing.Triangle], width, height) {
					return nil, fmt.Errorf(
						"pixel %d: positive UV overlap between triangles %d (id %d, UV %v) and %d (id %d, UV %v): %w",
						piece.pixel,
						existing.Triangle,
						triangles[existing.Triangle].ID,
						uv1[existing.Triangle],
						index,
						triangles[index].ID,
						triangleUV,
						errReceiver,
					)
				}
			}

			patches, patchErr := receiverClippedPatches(triangles[index], piece)
			if patchErr != nil {
				return nil, fmt.Errorf("pixel %d triangle %d precise patches: %w", piece.pixel, index, patchErr)
			}

			receiver.Patches = append(receiver.Patches, patches...)
			receiver.Pieces = append(receiver.Pieces, piece.piece)

			receiver.Coverage += piece.piece.Area
			if receiver.Coverage > 1.0001 {
				return nil, fmt.Errorf("pixel %d triangle %d: coverage %g: %w", piece.pixel, index, receiver.Coverage, errReceiver)
			}
		}
	}

	output := make([]Receiver, 0, len(byPixel))
	for _, receiver := range byPixel {
		if err := receiverGuide(triangles, receiver); err != nil {
			return nil, fmt.Errorf("pixel %d guide: %w", receiver.Pixel, err)
		}

		output = append(output, *receiver)
	}

	sort.Slice(output, func(i, j int) bool { return output[i].Pixel < output[j].Pixel })

	return output, nil
}

func TestReceiverRowsRetainExactSerialFootprints(t *testing.T) {
	t.Parallel()

	first := receiverFixtureTriangle()
	second := first
	second.ID = 2
	second.Positions = [3]Vec3{{1, 1, .25}, {1, 0, 0}, {0, 1, 0}}
	second.Normals = [3]Vec3{{0, 0, 1}, {.1, 0, 1}, {0, .2, 1}}

	cases := []struct {
		name          string
		triangles     []Triangle
		coordinates   [][3]Vec2
		width, height uint32
	}{
		{"shared-boundary", []Triangle{first, second}, [][3]Vec2{{{0, 0}, {1, 0}, {0, 1}}, {{1, 1}, {1, 0}, {0, 1}}}, 17, 31},
		{"sliver", []Triangle{first}, [][3]Vec2{{{.02, .02}, {.98, .02}, {.98, .021}}}, 37, 29},
		{"one-pixel", []Triangle{first, second}, [][3]Vec2{{{0, 0}, {1, 0}, {0, 1}}, {{1, 1}, {1, 0}, {0, 1}}}, 1, 1},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			expected, err := serialReceiverReference(fixture.triangles, fixture.coordinates, fixture.width, fixture.height)
			if err != nil {
				t.Fatal(err)
			}

			for _, workers := range []int{0, 1, 2, 7, 32} {
				actual, err := prepareReceiversWithWorkers(fixture.triangles, fixture.coordinates, fixture.width, fixture.height, workers)
				if err != nil || !reflect.DeepEqual(expected, actual) {
					t.Fatalf("workers %d changed exact serial footprints: %v", workers, err)
				}
			}
		})
	}
}

func TestReceiverRowsRetainOverlapAdmission(t *testing.T) {
	t.Parallel()

	first := receiverFixtureTriangle()
	second := first
	second.ID = 2

	coordinates := [][3]Vec2{{{.1, .1}, {.4, .1}, {.1, .4}}, {{.2, .1}, {.5, .1}, {.2, .4}}}
	for _, workers := range []int{1, 2, 7, 32} {
		if _, err := prepareReceiversWithWorkers([]Triangle{first, second}, coordinates, 17, 31, workers); err == nil {
			t.Fatal("parallel footprints accepted positive UV overlap", workers)
		}
	}
}

func TestReceiverPixelIndexDenseAndSparseBounds(t *testing.T) {
	t.Parallel()

	for _, index := range []receiverPixelIndex{
		newReceiverPixelIndex(100, 17, 31, 2, 10),
		newReceiverPixelIndex(2, math.MaxUint32, 1, 0, 1),
	} {
		pixel := index.base + 1
		if index.get(pixel) != 0 {
			t.Fatal("index did not start empty")
		}

		index.set(pixel, 7)

		if index.get(pixel) != 7 || index.get(pixel+1) != 0 {
			t.Fatal("pixel slot collided")
		}
	}

	if newReceiverPixelIndex(100, 17, 31, 2, 10).dense == nil {
		t.Fatal("bounded row did not use dense slots")
	}

	if newReceiverPixelIndex(2, math.MaxUint32, 1, 0, 1).dense != nil {
		t.Fatal("sparse admitted dimensions forced a page-sized allocation")
	}
}

func TestReceiverTrianglePatchSpansCannotExtendIntoNeighbors(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()

	receivers, err := prepareReceiversWithWorkers([]Triangle{triangle}, [][3]Vec2{triangle.UV}, 32, 32, 1)
	if err != nil || len(receivers) < 2 {
		t.Fatal(err)
	}

	first, second := &receivers[0], &receivers[1]

	expected := append([]ReceiverPatch(nil), second.Patches...)
	if cap(first.Patches) != len(first.Patches) {
		t.Fatal("borrowed first-piece span exposes neighboring arena elements")
	}

	first.Patches = append(first.Patches, ReceiverPatch{})

	first.Patches[len(first.Patches)-1].Positions[0][0] = 9
	if !reflect.DeepEqual(second.Patches, expected) {
		t.Fatal("receiver patch append mutated a neighboring receiver")
	}
}

func BenchmarkReceiverRows(b *testing.B) {
	first := receiverFixtureTriangle()
	second := first
	second.ID = 2
	second.Positions = [3]Vec3{{1, 1, 0}, {1, 0, 0}, {0, 1, 0}}
	triangles := []Triangle{first, second}
	coordinates := [][3]Vec2{{{0, 0}, {1, 0}, {0, 1}}, {{1, 1}, {1, 0}, {0, 1}}}

	for _, workers := range []int{1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("workers-%d", workers), func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				if _, err := prepareReceiversWithWorkers(triangles, coordinates, 256, 256, workers); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
