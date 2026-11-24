//go:build !js

//nolint:varnamelen // Fixture attribute comparisons use paired original/remapped vertices.
package lightmapbuild

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/lightmapdata"
	modelbuild "github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/xatlas"
)

func TestXatlasPreparedModelPreservesGeometryAndMaterialUVs(t *testing.T) {
	t.Parallel()

	input, err := modelbuild.Compile(testfixture.Scene(), "fixture.glb")
	if err != nil {
		t.Fatal(err)
	}

	before, err := modeldata.Decode(input.Data)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := prepareFixture(ctx, input.Data, Options{Resolution: 128, Padding: 3})
	if err != nil {
		t.Fatal(err)
	}

	if result.Width != 128 || result.Height != 128 {
		t.Fatalf("requested one 128-square page, got %dx%d", result.Width, result.Height)
	}

	after, err := modeldata.Decode(result.Model)
	if err != nil {
		t.Fatal(err)
	}

	bake, err := lightmapdata.Decode(result.Lightmap)
	if err != nil {
		t.Fatal(err)
	}

	if bake.ModelSHA256 != sha256.Sum256(result.Model) || len(before.Parts) != len(after.Parts) || len(bake.UVs) != len(after.Parts) ||
		result.CoveredTexels == 0 {
		t.Fatal("invalid artifact binding or empty receivers")
	}

	for i, a := range after.Parts {
		b := before.Parts[i]
		if a.Name != b.Name || a.Material != b.Material || len(a.Indices) != len(b.Indices) || len(bake.UVs[i])*4 != len(a.Vertices) {
			t.Fatal("changed model primitive metadata")
		}
		// xatlas remaps vertex indices but retains triangle order and all
		// position/normal/UV0 attributes associated with each corner.
		for corner := range len(a.Indices) / 4 {
			offset := corner * 4
			ai := int(binary.LittleEndian.Uint32(a.Indices[offset:])) * 32

			bi := int(binary.LittleEndian.Uint32(b.Indices[offset:])) * 32
			if !bytes.Equal(a.Vertices[ai:ai+32], b.Vertices[bi:bi+32]) {
				t.Fatalf("part %d corner %d changed source attributes", i, offset/4)
			}
		}
	}

	for i := range len(bake.Padding) / 4 {
		source := binary.LittleEndian.Uint32(bake.Padding[i*4:])
		if source == math.MaxUint32 {
			continue
		}

		if readFloat(bake.Receivers[int(source)*32+12:]) == 0 {
			t.Fatal("padding references uncovered sample")
		}
	}
}

func TestBVHAdjacentChildrenAndCompleteLeafCoverage(t *testing.T) {
	t.Parallel()

	triangles := make([]triangle, 65)
	for i := range triangles {
		x := float32(i)
		triangles[i] = triangle{points: [3]vec{{x, 0, 0}, {x + 1, 0, 0}, {x, 1, 0}}, center: vec{x + .333, .333, 0}, ordinal: i}
	}

	nodes, packed := buildBVH(triangles)
	visited := make([]bool, len(triangles))

	var walk func(uint32)

	walk = func(index uint32) {
		if uint64(index)*32 >= uint64(len(nodes)) {
			t.Fatal("child out of bounds")
		}

		data := nodes[int(index)*32:]

		first, count := binary.LittleEndian.Uint32(data[12:]), binary.LittleEndian.Uint32(data[28:])
		if count == 0 {
			walk(first)
			walk(first + 1)

			return
		}

		for i := first; i < first+count; i++ {
			if int(i) >= len(visited) || visited[i] {
				t.Fatal("invalid or duplicate leaf triangle")
			}

			visited[i] = true
		}
	}
	walk(0)

	for _, v := range visited {
		if !v {
			t.Fatal("missing leaf triangle")
		}
	}

	if len(packed) != len(triangles)*48 {
		t.Fatal("wrong triangle packing")
	}
}

func TestPaddingIncludesDiagonalGuttersAndKeepsChartOwnership(t *testing.T) {
	t.Parallel()

	atlas := xatlas.Atlas{Width: 5, Height: 5, Charts: 2, Image: make([]uint32, 25)}

	owners := make([]int32, 25)
	for i := range owners {
		owners[i] = -1
		atlas.Image[i] = 0x80000000
	}

	owners[12] = 0
	atlas.Image[4] = 0x80000001

	lookup, err := paddingLookup(context.Background(), atlas, owners)
	if err != nil {
		t.Fatal(err)
	}

	for _, index := range []int{0, 20, 24} {
		if binary.LittleEndian.Uint32(lookup[index*4:]) != 12 {
			t.Fatalf("diagonal gutter %d missing", index)
		}
	}

	if binary.LittleEndian.Uint32(lookup[16:]) != math.MaxUint32 {
		t.Fatal("padding crossed into a different chart")
	}
}

func TestPaddingFillsThinChartRegionsBeyondPackingGutter(t *testing.T) {
	t.Parallel()

	// A thin chart tail can cover atlas pixels without containing a texel
	// center on the surface. Its lighting must reach the nearest surface
	// receiver even when the tail exceeds the packer's gutter width.
	atlas := xatlas.Atlas{Width: 16, Height: 1, Charts: 2, Image: make([]uint32, 16)}
	owners := make([]int32, len(atlas.Image))

	for index := range owners {
		owners[index] = -1
		atlas.Image[index] = 0x80000000
	}

	owners[0] = 0
	atlas.Image[15] = 0x80000001
	owners[15] = 1

	lookup, err := paddingLookup(context.Background(), atlas, owners)
	if err != nil {
		t.Fatal(err)
	}

	for index := range 15 {
		if got := binary.LittleEndian.Uint32(lookup[index*4:]); got != 0 {
			t.Fatalf("thin chart pixel %d has receiver %d, want 0", index, got)
		}
	}

	if got := binary.LittleEndian.Uint32(lookup[15*4:]); got != 15 {
		t.Fatalf("adjacent chart has receiver %d, want 15", got)
	}
}
