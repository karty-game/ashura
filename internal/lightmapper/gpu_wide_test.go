//go:build !js

//nolint:varnamelen // Wire corruption and capacity fixtures use specified field positions and power-of-eight topology bounds.
package lightmapper

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
)

func gpuWideTestScene(t *testing.T, count int) *PreparedScene {
	t.Helper()

	source := testCoreScene()
	source.Materials = []Material{{DoubleSided: true}}

	for id := range count {
		x, y := float32(id%17)*3, float32(id/17)*3
		source.Triangles = append(
			source.Triangles,
			Triangle{ID: uint64(id + 1), Positions: [3]Vec3{{x, y, 0}, {x + 1, y, 0}, {x, y + 1, 0}}},
		)
	}

	scene, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	return scene
}

func TestGPUWideConstructionAndOctantCoverage(t *testing.T) {
	t.Parallel()

	for _, count := range []int{0, 1, 2, 3, 4, 7, 8, 9, 23, 24, 25, 193, 2049} {
		scene := gpuWideTestScene(t, count)

		first, err := gpuBuildWide(scene, gpuMaximumBytes/gpuWordBytes)
		if err != nil {
			t.Fatal(count, err)
		}

		second, err := gpuBuildWide(scene, gpuMaximumBytes/gpuWordBytes)
		if err != nil {
			t.Fatal(count, err)
		}

		if !slices.Equal(first.packed, second.packed) || !slices.Equal(first.order, second.order) {
			t.Fatal("non deterministic wide tree", count)
		}

		if first.linear || first.depth > gpuWideMaximumDepth {
			t.Fatal("ordinary scene selected fallback", count)
		}

		if len(first.packed) > 0 {
			assertPackedGPUWideCoverage(t, first)
		}

		packet, err := PackGPUScene(scene)
		if err != nil || !gpuValidateWideScene(packet.Data) {
			t.Fatal("invalid produced tree", count, err)
		}
	}
}

//nolint:gosec // G602: largest is selected only from the initialized prefix 0..used-1; every admitted count is independently enumerated.
func TestGPUWideBalancedCapacityProof(t *testing.T) {
	t.Parallel()

	for count := 4; count <= 1<<20; count++ {
		parts := [8]int{count}

		used := 1
		for used < 8 {
			largest := -1
			for i := range used {
				if parts[i] > 3 && (largest < 0 || parts[i] > parts[largest]) {
					largest = i
				}
			}

			if largest < 0 {
				break
			}

			n := parts[largest]
			parts[largest], parts[used] = n/2, n-n/2
			used++
		}

		sum := 0
		for _, n := range parts[:used] {
			sum += n
			if n > 3 && n > (count+7)/8 {
				t.Fatal("unbounded balanced frontier", count, n)
			}
		}

		if sum != count {
			t.Fatal("lost primitives", count)
		}
	}

	count, depth := 1<<20, 1
	for count > 3 {
		count = (count + 7) / 8
		depth++
	}

	if depth > 8 {
		t.Fatal("balanced fallback depth", depth)
	}
}

func TestGPUWideLinearFallbackUsesActualByteBudget(t *testing.T) {
	t.Parallel()
	scene := gpuWideTestScene(t, 193)

	build, err := gpuBuildWide(scene, 0)
	if err != nil {
		t.Fatal(err)
	}

	if !build.linear || len(build.packed) != 0 || build.depth != 0 || !slices.Equal(build.order, scene.order) {
		t.Fatal("invalid linear fallback")
	}
}

func TestGPUWideCompleteAdmissionRejectsCorruption(t *testing.T) {
	t.Parallel()
	scene := gpuWideTestScene(t, 73)

	packet, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	word := func(at int) uint32 { return binary.LittleEndian.Uint32(packet.Data[at*4:]) }
	nodes, as := int(word(2)), int(word(33))

	changes := []func([]byte){
		func(data []byte) { binary.LittleEndian.PutUint32(data[4*4:], 1) },
		func(data []byte) { binary.LittleEndian.PutUint32(data[34*4:], 1) },
		func(data []byte) { binary.LittleEndian.PutUint32(data[gpuWideDepthWord*4:], 12) },
		func(data []byte) { binary.LittleEndian.PutUint32(data[(nodes+4)*4:], ^uint32(0)) },
		func(data []byte) {
			for at := nodes + 14; at < nodes+20; at++ {
				binary.LittleEndian.PutUint32(data[at*4:], 0)
			}
		},
		func(data []byte) { binary.LittleEndian.PutUint32(data[(as+12+7)*4:], word(as+7)) },
		func(data []byte) { binary.LittleEndian.PutUint32(data[(as+12+11)*4:], word(as+11)) },
		func(data []byte) { binary.LittleEndian.PutUint32(data[(as+4)*4:], word(as+4)^1) },
		func(data []byte) { binary.LittleEndian.PutUint32(data[38*4:], 1) },
	}
	for index, change := range changes {
		bad := append([]byte(nil), packet.Data...)
		change(bad)

		if gpuValidateWideScene(bad) {
			t.Fatal("admitted malformed wide scene", index)
		}
	}
}

func TestGPUWideUnrepresentableDecodedProductUsesLinearLayout(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{DoubleSided: true}}

	for id := range 4 {
		x := float32(3e38)
		if id%2 == 0 {
			x = -x
		}

		next := math.Nextafter32(x, 0)
		source.Triangles = append(
			source.Triangles,
			Triangle{ID: uint64(id + 1), Positions: [3]Vec3{{x, 0, 0}, {next, 0, 0}, {x, 1e-32, 0}}},
		)
	}

	scene, err := PrepareScene(source)
	if err != nil {
		t.Fatal("source previously admitted", err)
	}

	packet, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal("lost admitted source", err)
	}

	word := func(at int) uint32 { return binary.LittleEndian.Uint32(packet.Data[at*4:]) }
	if word(gpuWideModeWord) != 1 || word(3) != 0 || word(gpuWideDepthWord) != 0 || !gpuValidateWideScene(packet.Data) {
		t.Fatal("invalid representability fallback")
	}
}

func TestGPUWideDeepSAHUsesBalancedTopology(t *testing.T) {
	t.Parallel()

	const count = 99

	build := &gpuWideBuilder{
		scene:  &PreparedScene{source: Scene{Triangles: make([]Triangle, count)}},
		packed: make([]uint32, gpuNodeWords),
	}
	for id := range count {
		build.oldOrder = append(build.oldOrder, uint32(id))
	}
	// Coincident zero-area structural bounds force an arbitrarily deep SAH tie;
	// this tests topology scheduling independently of source-surface admission.
	var chain func([]uint32) int

	chain = func(ids []uint32) int {
		if len(ids) <= 3 {
			return build.micro(ids)
		}

		index := len(build.nodes)
		build.nodes = append(build.nodes, gpuWideNode{b: build.bounds(ids), count: len(ids)})
		left, right := build.micro(ids[:3]), chain(ids[3:])
		build.nodes[index].left, build.nodes[index].right = left, right

		return index
	}

	root := chain(build.oldOrder)
	if err := build.buildBounded(root, gpuWideMaximumDepth, gpuMaximumBytes/gpuWordBytes); err != nil {
		t.Fatal(err)
	}

	if !build.balanced || build.linear || build.depth > 8 {
		t.Fatal("invalid depth fallback", build.depth)
	}

	assertPackedGPUWideCoverage(t, build)
}
