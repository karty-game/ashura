//go:build !js

package lightmapbuild

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestSAHBVHAdjacencyCoverageAndMaterialAssociation(t *testing.T) {
	t.Parallel()

	model := modeldata.Document{Source: "SAH planes"}

	for index := range 32 {
		part := receiverQuad("plane", float32(index), false)
		part.Material.Color[0] = float32(index+1) / 40
		model.Parts = append(model.Parts, part)
	}

	original := collectTriangles(model.Parts)
	triangles := append([]triangle(nil), original...)

	nodes, positions, err := buildSAHBVH(triangles)
	if err != nil {
		t.Fatal(err)
	}

	seen := make([]bool, len(triangles))
	maximumDepth := 0

	var visit func(uint32, int)

	visit = func(index uint32, depth int) {
		if int(index) >= len(nodes)/32 || depth >= 63 {
			t.Fatal("invalid adjacency/depth")
		}

		maximumDepth = max(maximumDepth, depth)
		data := nodes[int(index)*32:]

		first, count := binary.LittleEndian.Uint32(data[12:]), binary.LittleEndian.Uint32(data[28:])
		if count == 0 {
			visit(first, depth+1)
			visit(first+1, depth+1)

			return
		}

		if count > 8 || int(first+count) > len(seen) {
			t.Fatal("leaf escaped triangle bounds")
		}

		for i := first; i < first+count; i++ {
			if seen[i] {
				t.Fatal("duplicate leaf triangle")
			}

			seen[i] = true
		}
	}
	visit(0, 0)

	for _, value := range seen {
		if !value {
			t.Fatal("missing triangle")
		}
	}

	surfaces, err := buildSurfaces(triangles, model)
	if err != nil {
		t.Fatal(err)
	}

	scene := newTransportScene(nodes, triangles, surfaces)

	hit, distance := scene.trace(vec{.25, 40, .25}, vec{0, -1, 0})
	if hit < 0 || distance != 9 || scene.triangles[hit].surface[0] != float32(32)/40 {
		t.Fatal("nearest material association changed", hit, distance)
	}

	again := append([]triangle(nil), original...)

	otherNodes, otherPositions, err := buildSAHBVH(again)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(nodes, otherNodes) || !bytes.Equal(positions, otherPositions) {
		t.Fatal("SAH is nondeterministic")
	}

	t.Logf("nodes=%d depth=%d", len(nodes)/32, maximumDepth)
}

func TestSAHTraversalMatchesBruteForce(t *testing.T) {
	t.Parallel()

	_, rays := transportTraversalFixture()

	source := make([]triangle, 300)
	for i := range source {
		x, z := float32(i%20), float32(i/20)
		source[i] = triangle{
			points:  [3]vec{{x, 0, z}, {x + .8, 0, z}, {x, 1, z + .8}},
			center:  vec{x + .8/3, 1.0 / 3, z + .8/3},
			ordinal: i,
		}
	}

	nodes, _, err := buildSAHBVH(source)
	if err != nil {
		t.Fatal(err)
	}

	scene := newTransportScene(nodes, source, make([]byte, len(source)*16))
	for _, ray := range rays {
		_, distance := scene.trace(ray[0], ray[1])
		brute := float32(math.Inf(1))

		for _, triangle := range scene.triangles {
			d := transportIntersection(triangle, ray[0], ray[1])
			if d > 1e-4 && d < brute {
				brute = d
			}
		}

		if distance != brute {
			t.Fatal("SAH missed closest geometry", distance, brute)
		}
	}
}
