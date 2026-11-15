//go:build !js

package xatlas

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestWASIRepairsOverlapsPreservingValidChartsAndSourceFaces(t *testing.T) {
	t.Parallel()

	session := wasiSession{}
	defer func() { _ = session.close() }()

	if err := session.prepare(t.Context()); err != nil {
		t.Fatal(err)
	}

	module, err := session.runtime.InstantiateModule(
		t.Context(),
		session.compiled,
		wazero.NewModuleConfig().WithName("").WithStartFunctions(),
	)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = module.Close(context.Background()) }()

	guest := atlasGuest{t.Context(), module}
	coordinates := []float32{0, 0, 1, 0, 0, 1, .2, .5, 2, 0, 3, 0, 2, 1}
	indices := []uint32{0, 1, 2, 0, 1, 3, 4, 5, 6}

	var uvBytes, indexBytes, materialBytes, source []byte
	for _, value := range coordinates {
		uvBytes = binary.LittleEndian.AppendUint32(uvBytes, math.Float32bits(value))
	}

	for _, index := range indices {
		indexBytes = binary.LittleEndian.AppendUint32(indexBytes, index)
	}

	for _, value := range []uint32{0, 0, 1} {
		materialBytes = binary.LittleEndian.AppendUint32(materialBytes, value)
	}

	for vertexIndex := range 7 {
		z := float32(0)
		if vertexIndex == 3 {
			z = .5
		}

		for _, v := range []float32{coordinates[vertexIndex*2], coordinates[vertexIndex*2+1], z, 0, 0, 1, 0, 0} {
			source = binary.LittleEndian.AppendUint32(source, math.Float32bits(v))
		}
	}

	atlas := guest.call("xatlasCreate")
	vp, ip, mp, decl := guest.copy(uvBytes), guest.copy(indexBytes), guest.copy(materialBytes), guest.alloc(32)
	guest.call("xatlasUvMeshDeclInit", uint64(decl))

	for _, field := range [][2]uint32{{0, vp}, {4, ip}, {8, mp}, {12, 7}, {16, 8}, {20, 9}, {28, 1}} {
		guest.put(decl+field[0], field[1])
	}

	if guest.call("xatlasAddUvMesh", uint64(atlas), uint64(decl)) != 0 {
		t.Fatal("fixture mesh rejected")
	}

	pack := guest.alloc(24)
	guest.call("xatlasPackOptionsInit", uint64(pack))
	guest.put(pack+4, 2)
	guest.putf(pack+8, 8)
	guest.put(pack+12, 64)
	guest.write(pack+19, []byte{1})
	guest.call("xatlasComputeCharts", uint64(atlas), 0)
	guest.call("xatlasPackCharts", uint64(atlas), uint64(pack))
	before := guest.atlas(atlas, 1, 8, 64)

	overlap := false
	for _, faces := range before.meshes[0].charts {
		overlap = overlap || chartOverlaps(t.Context(), before.meshes[0], faces)
	}

	if !overlap {
		t.Fatal("fixture no longer exercises overlapping charts")
	}

	lengths := func(mesh rawMesh) (float64, float64) {
		first, second, third := mesh.vertices[mesh.indices[6]], mesh.vertices[mesh.indices[7]], mesh.vertices[mesh.indices[8]]

		return math.Hypot(
				float64(first.uv[0]-second.uv[0]),
				float64(first.uv[1]-second.uv[1]),
			), math.Hypot(
				float64(first.uv[0]-third.uv[0]),
				float64(first.uv[1]-third.uv[1]),
			)
	}
	firstLength, secondLength := lengths(before.meshes[0])

	after := guest.repair(atlas, pack, before, [][]byte{source}, 8, 1, 8, 64)
	if after.charts != before.charts+1 || len(after.meshes[0].indices) != 9 {
		t.Fatal("repair changed triangle membership/chart split")
	}

	mesh := after.meshes[0]
	for corner, index := range mesh.indices {
		if mesh.vertices[index].original != indices[corner] {
			t.Fatal("source corner remap changed")
		}
	}

	for _, faces := range mesh.charts {
		if chartOverlaps(t.Context(), mesh, faces) {
			t.Fatal("repair retained positive overlap")
		}
	}

	actualAB, actualAC := lengths(mesh)
	if math.Abs(actualAB-firstLength) > 1e-4 || math.Abs(actualAC-secondLength) > 1e-4 {
		t.Fatal("valid chart UV lengths changed")
	}
}
