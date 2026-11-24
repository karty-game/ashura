//go:build !js

package meshopt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/karty-game/ashura/internal/modeldata"
)

func fixtureVertex(x, y, u float32) []byte {
	var data []byte
	for _, v := range []float32{x, y, 0, 0, 0, 1, u, y} {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(v))
	}

	return data
}
func fixtureIndices(values ...uint32) []byte {
	var data []byte
	for _, v := range values {
		data = binary.LittleEndian.AppendUint32(data, v)
	}

	return data
}
func fixturePart() modeldata.Part {
	var vertices []byte
	for _, vertex := range [][]byte{fixtureVertex(0, 0, 0), fixtureVertex(1, 0, 1), fixtureVertex(0, 1, 0), fixtureVertex(0, 0, 0), fixtureVertex(0, 0, .5), fixtureVertex(8, 8, 8), fixtureVertex(2, 0, 2)} {
		vertices = append(vertices, vertex...)
	}

	return modeldata.Part{
		Name:     "seams",
		Material: modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Albedo: -1, Normal: -1, AO: -1},
		Vertices: vertices,
		Indices:  fixtureIndices(0, 1, 2, 3, 1, 2, 4, 1, 2, 0, 0, 1, 0, 1, 6),
	}
}

func TestCleanupPreservesSeamsMaterialsAndOrientedFaces(t *testing.T) {
	t.Parallel()

	part := fixturePart()
	source := modeldata.Document{Source: "fixture", Parts: []modeldata.Part{part, part}}
	before := slices.Clone(part.Vertices)

	output, err := Cleanup(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	if len(output.Parts) != 2 {
		t.Fatal("merged material boundaries")
	}

	for _, result := range output.Parts {
		if result.Name != part.Name || result.Material != part.Material || len(result.Vertices) != 4*32 || len(result.Indices) != 9*4 {
			t.Fatal("incorrect cleanup", len(result.Vertices), len(result.Indices))
		}

		if !reflect.DeepEqual(corners(result), corners(modeldata.Part{Vertices: part.Vertices, Indices: part.Indices[:9*4]})) {
			t.Fatal("changed UV seam, normal, winding or surviving face")
		}
	}

	if !bytes.Equal(before, part.Vertices) || len(source.Parts[0].Indices) != 15*4 {
		t.Fatal("mutated input")
	}

	bad := part

	bad.Indices = fixtureIndices(0, 0, 0)
	if _, err := Cleanup(t.Context(), modeldata.Document{Parts: []modeldata.Part{bad}}); err == nil {
		t.Fatal("accepted an empty cleaned model")
	}
}

func corners(part modeldata.Part) []string {
	var triangles []string

	for offset := 0; offset < len(part.Indices); offset += 12 {
		var face []byte

		for c := range 3 {
			index := int(binary.LittleEndian.Uint32(part.Indices[offset+c*4:]))
			face = append(face, part.Vertices[index*32:(index+1)*32]...)
		}

		triangles = append(triangles, string(face))
	}

	slices.Sort(triangles)

	return triangles
}

func gridPart() modeldata.Part {
	part := fixturePart()
	part.Vertices = nil
	part.Indices = nil

	const size = 24
	for y := range size + 1 {
		for x := range size + 1 {
			part.Vertices = append(part.Vertices, fixtureVertex(float32(x), float32(y), float32(x))...)
		}
	}

	ordered := make([]uint32, 0, 6*size*size)

	for y := range size {
		for x := range size {
			a := uint32(y*(size+1) + x)
			ordered = append(ordered, a, a+1, a+size+1, a+1, a+size+2, a+size+1)
		}
	}

	triangles := len(ordered) / 3
	for i := range triangles {
		offset := (i * 337 % triangles) * 3
		for _, v := range ordered[offset : offset+3] {
			part.Indices = binary.LittleEndian.AppendUint32(part.Indices, v)
		}
	}

	return part
}

func cacheMisses(indices []byte) int {
	var cache []uint32

	misses := 0

	for i := 0; i < len(indices); i += 4 {
		vertex := binary.LittleEndian.Uint32(indices[i:])

		at := slices.Index(cache, vertex)
		if at < 0 {
			misses++
		} else {
			cache = slices.Delete(cache, at, at+1)
		}

		cache = append([]uint32{vertex}, cache...)
		if len(cache) > 16 {
			cache = cache[:16]
		}
	}

	return misses
}

func TestWASMOrderingImprovesCacheAndKeepsExactGeometry(t *testing.T) {
	t.Parallel()

	part := gridPart()
	source := modeldata.Document{Source: "grid", Parts: []modeldata.Part{part}}

	output, err := Optimize(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	after := output.Parts[0]
	if !reflect.DeepEqual(corners(part), corners(after)) {
		t.Fatal("changed oriented corner attributes")
	}

	before, improved := cacheMisses(part.Indices), cacheMisses(after.Indices)
	if improved >= before {
		t.Fatal("cache misses did not decrease", before, improved)
	}

	t.Logf("LRU16 misses: %d -> %d; vertices=%d triangles=%d", before, improved, len(after.Vertices)/32, len(after.Indices)/12)

	again, err := Optimize(t.Context(), source)
	if err != nil || !bytes.Equal(after.Vertices, again.Parts[0].Vertices) || !bytes.Equal(after.Indices, again.Parts[0].Indices) {
		t.Fatal("nondeterministic optimization", err)
	}
}

func TestMalformedGeometryAndCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Optimize(ctx, modeldata.Document{Parts: []modeldata.Part{fixturePart()}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	if _, err := Cleanup(ctx, modeldata.Document{Parts: []modeldata.Part{fixturePart()}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	for _, mutate := range []func(*modeldata.Part){func(p *modeldata.Part) { p.Indices = fixtureIndices(999, 0, 1) }, func(p *modeldata.Part) { binary.LittleEndian.PutUint32(p.Vertices, 0x7f800000) }} {
		part := fixturePart()
		mutate(&part)

		doc := modeldata.Document{Parts: []modeldata.Part{part}}
		if _, err := Cleanup(t.Context(), doc); err == nil {
			t.Fatal("cleanup admitted invalid geometry")
		}

		if _, err := Optimize(t.Context(), doc); err == nil {
			t.Fatal("optimizer admitted invalid geometry")
		}
	}

	//nolint:staticcheck // Explicit invalid-context admission fixture.
	if _, err := Open(nil); err == nil {
		t.Fatal("nil context admitted")
	}
}
