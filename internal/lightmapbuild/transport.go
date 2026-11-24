//go:build !js

//nolint:mnd,gosec // GPU ABI offsets and conversions are bounded by 2048-square atlases and the 128 MiB transport limit.
package lightmapbuild

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"runtime"
	"slices"
	"sync"

	"github.com/karty-game/ashura/internal/lightmapdata"
)

// Transport is a fixed-geometry, single diffuse bounce approximation. Samples
// coalesce into 0.9m physical cells, dominant geometric-normal bin and material
// reflectance. Keeping the strongest eight links rescales each RNM lobe's retained
// sum, preserving energy while approximating the distribution of weak reflectors.
const transportCellSize = .9

// The page compiler may spill a portable-buffer overflow into additional pages.
// Other geometry/admission errors retain their ordinary fail-fast behavior.
var errTransportBudget = fmt.Errorf("prepared lightmap page exceeds resource budget: %w", errGeometry)

type transportKey struct {
	cell          [3]uint64
	material      [3]uint32
	normal, sided uint32
}
type transportGroup struct {
	key                                   transportKey
	position, normal, reflectance, weight vec
}
type transportReceiver struct {
	sky, ground vec
	count       int
	links       [8]transportGroup
}

func buildTransport(ctx context.Context, document *lightmapdata.Document, triangles []triangle, geometric []vec, samples uint32) error {
	return buildTransportMode(ctx, document, triangles, geometric, samples, false)
}

func buildTransportMode(
	ctx context.Context,
	document *lightmapdata.Document,
	triangles []triangle,
	geometric []vec,
	samples uint32,
	ordered bool,
) error {
	scene := newTransportScene(document.Nodes, triangles, document.Surfaces)
	scene.ordered = ordered
	covered := initializeTransport(document, samples)

	if err := transportSize(document); err != nil {
		return err
	}

	directions := transportDirections(int(samples))
	patches := make(map[transportKey]uint32)

	const batchSize = 512

	results := make([]transportReceiver, batchSize)
	for start := 0; start < len(covered); start += batchSize {
		count := min(batchSize, len(covered)-start)
		if err := transportParallel(ctx, count, func(worker, index int, groups map[transportKey]int) {
			pixel := covered[start+index]
			results[index] = sampleTransportReceiver(scene, document, pixel, geometric[pixel], directions, groups)
		}); err != nil {
			return err
		}

		for index := range count {
			appendTransportReceiver(document, start+index, results[index], patches)
		}

		if err := transportSize(document); err != nil {
			return err
		}
	}

	return transportParallel(ctx, len(document.GIPatches)/48, func(worker, index int, _ map[transportKey]int) {
		data := document.GIPatches[index*48:]
		position := vec{readFloat(data), readFloat(data[4:]), readFloat(data[8:])}
		normal := vec{readFloat(data[16:]), readFloat(data[20:]), readFloat(data[24:])}
		sky, ground := patchEnvironment(scene, position, normal, directions)
		putFloat(data[12:], sky)
		putFloat(data[28:], ground)
	})
}

func transportSize(document *lightmapdata.Document) error {
	size := len(
		document.Nodes,
	) + len(
		document.Triangles,
	) + len(
		document.Receivers,
	) + len(
		document.Padding,
	) + len(
		document.Frames,
	) + len(
		document.Surfaces,
	) + len(
		document.ReceiverGIIndex,
	) + len(
		document.GIReceivers,
	) + len(
		document.GIPatches,
	) + len(
		document.GITransfers,
	)
	if size > lightmapdata.MaxTransportBytes {
		return fmt.Errorf(
			"prepared diffuse transport needs %d GPU bytes, exceeds %d-byte storage limit: %w",
			size,
			lightmapdata.MaxTransportBytes,
			errTransportBudget,
		)
	}

	return nil
}

func transportParallel(ctx context.Context, count int, run func(int, int, map[transportKey]int)) error {
	workers := min(2, runtime.GOMAXPROCS(0))

	var wait sync.WaitGroup
	for worker := range workers {
		wait.Go(func() {
			groups := make(map[transportKey]int, 128)

			for index := worker; index < count; index += workers {
				if index%32 == worker && ctx.Err() != nil {
					return
				}

				clear(groups)
				run(worker, index, groups)
			}
		})
	}

	wait.Wait()

	return ctx.Err()
}

func sampleTransportReceiver(
	scene transportScene,
	document *lightmapdata.Document,
	pixel int,
	geometric vec,
	directions []vec,
	lookup map[transportKey]int,
) transportReceiver {
	position, basis := receiverTransportBasis(document, pixel)

	var (
		result transportReceiver
		groups [128]transportGroup
	)

	count := 0
	total := vec{}
	origin := add(position, scale(geometric, .005))

	hemisphereTangent, hemisphereBitangent := hemisphereFrame(geometric)
	for _, sample := range directions {
		direction := add(add(scale(hemisphereTangent, sample[0]), scale(hemisphereBitangent, sample[1])), scale(geometric, sample[2]))

		weight := vec{}
		for axis := range 3 {
			weight[axis] = 2 / float32(len(directions)) * max(0, dot(basis[axis], direction))
		}

		hit, distance := scene.trace(origin, direction)
		if hit < 0 {
			sky := max(0, min(1, .5+.5*direction[1]))
			result.sky = add(result.sky, scale(weight, sky))
			result.ground = add(result.ground, scale(weight, 1-sky))

			continue
		}

		triangle := scene.triangles[hit]

		facing := triangle.normal
		if dot(facing, direction) > 0 {
			if triangle.surface[3] == 0 {
				continue
			}

			facing = scale(facing, -1)
		}

		point := add(origin, scale(direction, distance))
		key := makeTransportKey(point, facing, triangle.surface)

		group, exists := lookup[key]
		if !exists {
			group = count
			count++
			lookup[key] = group
			groups[group] = transportGroup{
				key:         key,
				position:    point,
				normal:      facing,
				reflectance: vec{triangle.surface[0], triangle.surface[1], triangle.surface[2]},
			}
		}

		groups[group].weight = add(groups[group].weight, weight)
		total = add(total, weight)
	}

	retainTransportLinks(&result, groups[:count], total)

	return result
}

func makeTransportKey(position, normal vec, surface [4]float32) transportKey {
	key := transportKey{sided: uint32(surface[3])}
	axis := 0

	for candidate := range 3 {
		key.cell[candidate] = math.Float64bits(math.Floor(float64(position[candidate]) / transportCellSize))

		key.material[candidate] = math.Float32bits(surface[candidate])
		if math.Abs(float64(normal[candidate])) > math.Abs(float64(normal[axis])) {
			axis = candidate
		}
	}

	key.normal = uint32(axis * 2)
	if normal[axis] < 0 {
		key.normal++
	}

	return key
}

func transportDirections(count int) []vec {
	directions := make([]vec, count)
	for index := range directions {
		height := (float64(index) + .5) / float64(count)
		azimuth := 2 * math.Pi * float64(bits.Reverse32(uint32(index))) / 4294967296
		radius := math.Sqrt(1 - height*height)
		directions[index] = vec{float32(radius * math.Cos(azimuth)), float32(radius * math.Sin(azimuth)), float32(height)}
	}

	return directions
}

func hemisphereFrame(normal vec) (vec, vec) {
	axis := vec{0, 1, 0}
	if math.Abs(float64(normal[1])) > .9 {
		axis = vec{1, 0, 0}
	}

	tangent := normalize(cross(axis, normal))

	return tangent, cross(normal, tangent)
}

func patchEnvironment(scene transportScene, position, normal vec, directions []vec) (float32, float32) {
	sky, ground := float32(0), float32(0)
	origin := add(position, scale(normal, .005))

	tangent, bitangent := hemisphereFrame(normal)
	for _, sample := range directions {
		direction := add(add(scale(tangent, sample[0]), scale(bitangent, sample[1])), scale(normal, sample[2]))
		if hit, _ := scene.trace(origin, direction); hit >= 0 {
			continue
		}

		weight := 2 / float32(len(directions)) * sample[2]
		blend := max(0, min(1, .5+.5*direction[1]))
		sky += weight * blend
		ground += weight * (1 - blend)
	}

	return sky, ground
}

func add(a, b vec) vec { return vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

func initializeTransport(document *lightmapdata.Document, samples uint32) []int {
	covered := make([]int, 0, len(document.Receivers)/32)

	document.ReceiverGIIndex = make([]byte, len(document.Receivers)/8)
	for pixel := range len(document.Receivers) / 32 {
		index := uint32(math.MaxUint32)
		if readFloat(document.Receivers[pixel*32+12:]) > 0 {
			index = uint32(len(covered))
			covered = append(covered, pixel)
		}

		binary.LittleEndian.PutUint32(document.ReceiverGIIndex[pixel*4:], index)
	}

	document.GIReceivers = make([]byte, len(covered)*32)

	document.GISamples = samples

	return covered
}

func appendTransportReceiver(document *lightmapdata.Document, index int, receiver transportReceiver, patches map[transportKey]uint32) {
	output := document.GIReceivers[index*32:]
	binary.LittleEndian.PutUint32(output, uint32(len(document.GITransfers)/16))
	binary.LittleEndian.PutUint32(output[4:], uint32(receiver.count))

	for axis := range 3 {
		putFloat(output[8+axis*4:], receiver.sky[axis])
		putFloat(output[20+axis*4:], receiver.ground[axis])
	}

	for _, link := range receiver.links[:receiver.count] {
		patch, exists := patches[link.key]
		if !exists {
			patch = uint32(len(document.GIPatches) / 48)
			patches[link.key] = patch

			data := make([]byte, 48)
			for axis := range 3 {
				putFloat(data[axis*4:], link.position[axis])
				putFloat(data[16+axis*4:], link.normal[axis])
				putFloat(data[32+axis*4:], link.reflectance[axis])
			}

			putFloat(data[44:], float32(link.key.sided))
			document.GIPatches = append(document.GIPatches, data...)
		}

		data := make([]byte, 16)
		binary.LittleEndian.PutUint32(data, patch)

		for axis := range 3 {
			putFloat(data[4+axis*4:], link.weight[axis])
		}

		document.GITransfers = append(document.GITransfers, data...)
	}
}

func receiverTransportBasis(document *lightmapdata.Document, pixel int) (vec, [3]vec) {
	data := document.Receivers[pixel*32:]
	position := vec{readFloat(data), readFloat(data[4:]), readFloat(data[8:])}
	normal := vec{readFloat(data[16:]), readFloat(data[20:]), readFloat(data[24:])}
	frame := document.Frames[pixel*16:]
	tangent := vec{readFloat(frame), readFloat(frame[4:]), readFloat(frame[8:])}
	bitangent := scale(cross(normal, tangent), readFloat(frame[12:]))
	basis := [3]vec{
		add(scale(tangent, .81649658), scale(normal, .57735027)),
		add(add(scale(tangent, -.40824829), scale(bitangent, .70710678)), scale(normal, .57735027)),
		add(add(scale(tangent, -.40824829), scale(bitangent, -.70710678)), scale(normal, .57735027)),
	}

	return position, basis
}

func retainTransportLinks(result *transportReceiver, groups []transportGroup, total vec) {
	slices.SortStableFunc(groups, func(a, b transportGroup) int {
		first, second := dot(a.weight, vec{1, 1, 1}), dot(b.weight, vec{1, 1, 1})
		if first > second {
			return -1
		}

		if first < second {
			return 1
		}

		return 0
	})
	result.count = min(len(groups), len(result.links))
	copy(result.links[:], groups[:result.count])

	retained := vec{}
	for _, group := range result.links[:result.count] {
		retained = add(retained, group.weight)
	}

	if result.count > 0 {
		for axis := range 3 {
			if retained[axis] == 0 {
				result.links[0].weight[axis] = total[axis]

				continue
			}

			for index := range result.count {
				result.links[index].weight[axis] *= total[axis] / retained[axis]
			}
		}
	}
}
