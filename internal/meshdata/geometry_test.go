package meshdata_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/meshdata"
)

func TestAdmissionAndImmutableOwnership(t *testing.T) {
	t.Parallel()

	vertices := make([]byte, 3*meshdata.VertexStride)
	indices := []byte{0, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0}

	geometry, err := meshdata.New(vertices, indices)
	if err != nil {
		t.Fatal(err)
	}

	vertices[0], indices[0] = 255, 2

	if geometry.Vertices()[0] != 0 || geometry.Indices()[0] != 0 {
		t.Fatal("geometry retained caller-mutated data")
	}

	owned, err := meshdata.FromOwned(geometry.Vertices(), geometry.Indices())
	if err != nil {
		t.Fatal(err)
	}

	if &owned.Vertices()[0] != &geometry.Vertices()[0] || &owned.Indices()[0] != &geometry.Indices()[0] {
		t.Fatal("owned admission repacked geometry")
	}
}

func TestInvalidDataRejectedBeforeUse(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"empty", "stride", "triangle", "late-index", "position-nan", "uv-infinite"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			vertices := make([]byte, 3*meshdata.VertexStride)
			indices := []byte{0, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0}

			switch name {
			case "empty":
				vertices = nil
			case "stride":
				vertices = vertices[:len(vertices)-1]
			case "triangle":
				indices = indices[:8]
			case "late-index":
				binary.LittleEndian.PutUint32(indices[8:], 3)
			case "position-nan":
				binary.LittleEndian.PutUint32(vertices, math.Float32bits(float32(math.NaN())))
			case "uv-infinite":
				binary.LittleEndian.PutUint32(vertices[28:], math.Float32bits(float32(math.Inf(1))))
			}

			if _, err := meshdata.FromOwned(vertices, indices); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}
