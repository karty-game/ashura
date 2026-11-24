//go:build !js

package impostorfixture_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/karty-game/ashura/internal/impostorfixture"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modeldata"
)

func TestAuthoredTreeMatchesSourceAndCompiles(t *testing.T) {
	t.Parallel()

	data, stats, err := impostorfixture.Tree()
	if err != nil {
		t.Fatal(err)
	}

	retained, err := os.ReadFile("testdata/gnarled-tree.glb")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, retained) || stats.Triangles != 26740 || stats.Primitives != 4 || stats.Vertices != 17624 {
		t.Fatalf("source fixture or documented geometry counts changed: %+v", stats)
	}

	result, err := modelbuild.Compile(data, "testdata/gnarled-tree.glb")
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}

	if len(document.Parts) != 4 || len(document.Textures) != 0 || result.IndexBytes/12 != stats.Triangles {
		t.Fatalf("prepared tree lost geometry/material ownership: %+v", result)
	}

	for _, part := range document.Parts {
		if part.Material.Roughness != .85 || part.Material.Color[3] != 1 || part.Material.Albedo != -1 || part.Material.Normal != -1 ||
			part.Material.AO != -1 {
			t.Fatalf("unexpected source material: %+v", part.Material)
		}

		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			t.Fatal(err)
		}

		checkNormalsAndWinding(t, part)
	}
}

func checkNormalsAndWinding(t *testing.T, part modeldata.Part) {
	t.Helper()

	for offset := 0; offset < len(part.Vertices); offset += meshdata.VertexStride {
		normal := readVector(part.Vertices[offset+12:])

		length := dot(normal, normal)
		if math.Abs(length-1) > 1e-5 {
			t.Fatalf("source normal is not unit length: %v", normal)
		}
	}

	for offset := 0; offset < len(part.Indices); offset += 12 {
		var positions, normals [3][3]float64

		for corner := range 3 {
			index := int(binary.LittleEndian.Uint32(part.Indices[offset+corner*4:])) * meshdata.VertexStride
			positions[corner] = readVector(part.Vertices[index:])
			normals[corner] = readVector(part.Vertices[index+12:])
		}

		first, second := subtract(positions[1], positions[0]), subtract(positions[2], positions[0])

		face := [3]float64{
			first[1]*second[2] - first[2]*second[1],
			first[2]*second[0] - first[0]*second[2],
			first[0]*second[1] - first[1]*second[0],
		}
		if dot(face, normals[0])+dot(face, normals[1])+dot(face, normals[2]) < -1e-8 {
			t.Fatalf("source face winding opposes normals in %s triangle%d", part.Name, offset/12)
		}
	}
}

func readVector(data []byte) [3]float64 {
	return [3]float64{
		float64(math.Float32frombits(binary.LittleEndian.Uint32(data))),
		float64(math.Float32frombits(binary.LittleEndian.Uint32(data[4:]))),
		float64(math.Float32frombits(binary.LittleEndian.Uint32(data[8:]))),
	}
}

func subtract(left, right [3]float64) [3]float64 {
	return [3]float64{left[0] - right[0], left[1] - right[1], left[2] - right[2]}
}
func dot(left, right [3]float64) float64 {
	return left[0]*right[0] + left[1]*right[1] + left[2]*right[2]
}
