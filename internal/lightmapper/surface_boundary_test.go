package lightmapper

import (
	"math/rand/v2"
	"testing"
)

func TestCompactSurfaceBoundariesMatchHashOracle(t *testing.T) {
	t.Parallel()
	scene, receivers := surfaceEdgeFixture(t)

	patches, err := ReceiverPatches(scene, receivers[0])
	if err != nil {
		t.Fatal(err)
	}

	var scratch []surfaceBoundary

	for _, copies := range []int{1, 2, 3, 8} {
		var input []ReceiverPatch
		for range copies {
			input = append(input, patches...)
		}

		counts, records, oracleErr := surfacePatchSegments(input, 19)
		if oracleErr != nil {
			t.Fatal(oracleErr)
		}

		scratch, err = surfaceReceiverBoundary(input, 19, scratch)
		if err != nil || len(scratch) != len(counts) {
			t.Fatal(scratch, err)
		}

		for _, boundary := range scratch {
			if counts[boundary.key] != boundary.count || records[boundary.key] != boundary.segment {
				t.Fatal("changed exact footprint contact", boundary)
			}
		}
	}
}

func TestSurfaceBoundsNeverRejectExactPositiveContact(t *testing.T) {
	t.Parallel()
	//nolint:gosec // G404: seeded geometric property samples must be reproducible; no security-sensitive randomness is used.
	random := rand.New(rand.NewPCG(19, 37))
	for range 2000 {
		origin := Vec3{random.Float32()*20 - 10, random.Float32()*20 - 10, random.Float32()*20 - 10}
		axis := Normalize(Vec3{random.Float32(), random.Float32(), random.Float32()})
		normal := Normalize(Cross(axis, Vec3{1, 0, 0}))
		transverse := Cross(axis, normal)
		first := surfaceSegment{a: origin, b: Add(origin, axis), center: Add(origin, transverse), normal: normal}
		second := surfaceSegment{
			a:      Add(origin, Scale(axis, .1)),
			b:      Add(origin, Scale(axis, .9)),
			center: Sub(origin, transverse),
			normal: normal,
		}

		tolerance := surfaceTolerance(first.a, first.b, 0)
		if !surfaceSegmentsTouch(first, second, tolerance) {
			continue
		}

		if !surfaceBoundsOverlap(first, second, tolerance) {
			t.Fatal("broad phase removed an admitted contact", first, second, tolerance)
		}
	}
}
