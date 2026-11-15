//go:build !js

package lightmapper

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	wideHardwareRays   = 137
	wideHardwareOwners = 97
)

//nolint:paralleltest // Distinct rays in three workgroups exercise the production shared traversal stack on one device.
func TestHardwareGPUWideMixedRayPayloads(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	prepared := wideHardwareScene(t)
	packed := wideHardwarePacket(t, prepared)
	shader := wideHardwareShader(t)
	worker := newPayloadWorker(t, shader)
	rays := wideHardwareQueries()
	tile := wideHardwareQueryTile(rays)

	raw, err := worker.Run(t.Context(), packed, tile)
	if err != nil {
		t.Fatal("wide ray payloads", err)
	}

	for index, ray := range rays {
		assertWideHardwareRay(t, prepared, raw, index, ray)
	}
}

func wideHardwareShader(t *testing.T) string {
	t.Helper()

	shader := shadowPayloadShader(t)

	const entry = "let id=invocation.x;if id>=lp.count{return;}let ray=8u+id*8u;"
	if strings.Count(shader, entry) != 1 {
		t.Fatal("wide payload shader entry changed")
	}

	return strings.Replace(shader, entry, "lmTraceLane=invocation.x%64u;"+entry, 1)
}

//nolint:paralleltest // Compacted owner indirection and different pool widths must use actual invocation lanes, never owner modulo64.
func TestHardwareGPUWideCompactedSamplePools(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	scene := wideHardwareScene(t)
	packed := wideHardwarePacket(t, scene)
	receivers := wideHardwareReceivers()
	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 5, Bounces: 3, Seed: 29, RouletteDepth: 1}

	tile, err := PackGPUTile(scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	var (
		reference       []byte
		referenceCounts GPUTraceCounts
	)

	for _, profile := range []bool{false, true} {
		for _, lanes := range []uint64{1, 2, 5} {
			raw, counts := runSamplePoolTail(t, ctx, packed, tile, profile, lanes)
			if reference == nil {
				reference = bytes.Clone(raw)
				assertWideHardwareReference(t, ctx, scene, receivers, options, raw)
			}

			assertSamplePoolTail(t, raw, reference, tile)

			if profile {
				if referenceCounts.Rays == 0 {
					referenceCounts = counts
				}

				if counts != referenceCounts || counts.Rays <= wideHardwareOwners*uint64(options.Samples) || counts.ShadowRays == 0 {
					t.Fatal("wide compacted sample identity or continuation counts changed", lanes, counts, referenceCounts)
				}
			}
		}
	}
}

func wideHardwarePacket(t *testing.T, scene *PreparedScene) GPUScene {
	t.Helper()

	packed, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	if binary.LittleEndian.Uint32(packed.Data[3*gpuWordBytes:]) == 0 ||
		binary.LittleEndian.Uint32(packed.Data[36*gpuWordBytes:]) < 2 {
		t.Fatal("fixture did not construct a multilevel wide hierarchy")
	}

	return packed
}

// Six nested cubes provide overlapping internal bounds and different pending
// child masks for rays in every octant; a disjoint triangle grid needs no stack.
func wideHardwareScene(t *testing.T) *PreparedScene {
	t.Helper()

	source := Scene{
		Materials:   []Material{{Reflectance: Vec3{.6, .5, .4}, DoubleSided: true}},
		Emitters:    []Emitter{{ID: 1, Kind: Point, Position: Vec3{.3, 1, .2}, Intensity: Vec3{4, 3, 2}}},
		Environment: Environment{Sky: Vec3{.2, .15, .1}, Ground: Vec3{.04, .03, .02}},
	}

	for layer := range 6 {
		radius := 2 + float32(layer)*.35

		for axis := range 3 {
			for _, sign := range []float32{-1, 1} {
				source.Triangles = append(source.Triangles, wideHardwareFace(axis, sign, radius)...)
			}
		}
	}

	for index := range source.Triangles {
		source.Triangles[index].ID = uint64(index + 1)
	}

	return gpuFixtureScene(t, source)
}

func wideHardwareFace(axis int, sign, radius float32) []Triangle {
	var corners [4]Vec3
	for index := range corners {
		corners[index][axis] = sign * radius
		corners[index][(axis+1)%3] = -radius

		corners[index][(axis+2)%3] = -radius
		if index&1 != 0 {
			corners[index][(axis+1)%3] = radius
		}

		if index&2 != 0 {
			corners[index][(axis+2)%3] = radius
		}
	}

	var normal Vec3

	normal[axis] = -sign

	triangles := []Triangle{
		{Positions: [3]Vec3{corners[0], corners[1], corners[2]}},
		{Positions: [3]Vec3{corners[1], corners[3], corners[2]}},
	}
	for index := range triangles {
		if sign > 0 {
			triangles[index].Positions[1], triangles[index].Positions[2] = triangles[index].Positions[2], triangles[index].Positions[1]
		}

		triangles[index].Normals = [3]Vec3{normal, normal, normal}
	}

	return triangles
}

func wideHardwareQueries() []Ray {
	rays := make([]Ray, wideHardwareRays)
	for index := range rays {
		origin := Vec3{float32(index%17)*.03 - .24, float32(index%13)*.02 - .12, float32(index%11)*.04 - .2}
		direction := Vec3{.173 + float32(index%7)*.011, .327 + float32(index%5)*.013, .251 + float32(index%3)*.017}

		direction[index%3] = 1
		for axis := range direction {
			if index&(1<<axis) != 0 {
				direction[axis] = -direction[axis]
			}
		}

		limit := float32(100)

		switch index % 3 {
		case 0:
			limit = .5
		case 1:
			limit = 2.25
		}

		rays[index] = Ray{Origin: origin, Direction: direction, Min: 1e-6, Max: limit}
	}

	return rays
}

func wideHardwareQueryTile(rays []Ray) GPUTile {
	data := make([]byte, 32+len(rays)*32)
	//nolint:gosec // G115: the fixture has137 rays, represented by the admitted u32 count.
	count := uint32(len(rays))
	binary.LittleEndian.PutUint32(data, count)

	for index, ray := range rays {
		values := [8]float32{ray.Origin[0], ray.Origin[1], ray.Origin[2], ray.Max, ray.Direction[0], ray.Direction[1], ray.Direction[2], 0}
		for component, value := range values {
			binary.LittleEndian.PutUint32(data[32+index*32+component*gpuWordBytes:], math.Float32bits(value))
		}
	}

	return GPUTile{Data: data, Count: count, Options: Options{Samples: 1, Outputs: OutputRGB | OutputRNM, Seed: 29}}
}

func assertWideHardwareRay(t *testing.T, scene *PreparedScene, raw []byte, index int, ray Ray) {
	t.Helper()

	word := func(slot int) uint32 {
		return binary.LittleEndian.Uint32(raw[(index*gpuResultWords+slot)*gpuWordBytes:])
	}

	expected := scene.Intersect(ray, false)
	if word(33) != 1 || (word(0) != 0) != expected.Found {
		t.Fatal("wide ray visibility or completion", index, word(0), word(33), expected)
	}

	for _, offset := range []int{1, 10} {
		if !expected.Found {
			if word(offset) != math.MaxUint32 || word(offset+1) != math.MaxUint32 {
				t.Fatal("wide ray returned a false hit", index, offset)
			}

			continue
		}

		//nolint:gosec // G115: the prepared fixture contains exactly72 triangles.
		if word(offset) >= uint32(len(scene.source.Triangles)) || word(offset+1) != math.MaxUint32 {
			t.Fatal("wide complete hit lost identity", index, offset, word(offset), word(offset+1))
		}

		distance := math.Float32frombits(word(offset + 5))
		if !(distance >= ray.Min && distance < ray.Max) {
			t.Fatal("wide hit exceeded query interval", index, offset, distance)
		}

		if offset == 10 && (word(offset) != expected.Triangle || math.Abs(float64(distance-expected.Distance)) > 2e-5) {
			t.Fatal("wide closest hit differs from source reference", index, word(offset), distance, expected)
		}

		assertWideHardwareHit(t, scene.source.Triangles[word(offset)], word, offset, ray, distance)
	}
}

func assertWideHardwareHit(t *testing.T, triangle Triangle, word func(int) uint32, offset int, ray Ray, distance float32) {
	t.Helper()

	var normal, barycentric, position Vec3
	for axis := range 3 {
		normal[axis] = math.Float32frombits(word(offset + 2 + axis))
		barycentric[axis] = math.Float32frombits(word(offset + 6 + axis))
		position = Add(position, Scale(triangle.Positions[axis], barycentric[axis]))
	}

	wantNormal := Normalize(Cross(Sub(triangle.Positions[1], triangle.Positions[0]), Sub(triangle.Positions[2], triangle.Positions[0])))
	if normal != wantNormal || math.Abs(float64(barycentric[0]+barycentric[1]+barycentric[2]-1)) > 1e-5 {
		t.Fatal("wide complete hit lost normal or barycentrics", normal, wantNormal, barycentric)
	}

	point := Add(ray.Origin, Scale(ray.Direction, distance))
	for axis := range 3 {
		if barycentric[axis] < -1e-5 || math.Abs(float64(position[axis]-point[axis])) > 2e-5 {
			t.Fatal("wide complete hit does not lie on its source triangle", position, point, barycentric)
		}
	}
}

func wideHardwareReceivers() []Receiver {
	receivers := make([]Receiver, wideHardwareOwners)
	for index := range receivers {
		id := uint64(index+1)<<32 | uint64(wideHardwareOwners-index)
		x, z := float32(index%17)*.1-.8, float32(index/17)*.2-.5
		receivers[index] = gpuFixtureReceiver(id, x)

		receivers[index].Position[2] = z
		for corner := range receivers[index].Patches[0].Positions {
			receivers[index].Patches[0].Positions[corner][2] += z
		}
	}

	return receivers
}

func assertWideHardwareReference(
	t *testing.T,
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	raw []byte,
) {
	t.Helper()

	estimates, err := DecodeGPUEstimates(raw, receivers)
	if err != nil {
		t.Fatal(err)
	}

	reference, err := BakeReference(ctx, scene, receivers, options)
	if err != nil {
		t.Fatal(err)
	}

	gpuCompare(t, Result{Estimates: estimates}, reference, .003)
}
