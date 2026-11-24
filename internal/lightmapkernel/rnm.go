package lightmapkernel

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"math"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/pkg/compute"
	"github.com/karty-game/ashura/pkg/shader"
)

const (
	RNMUniformSize       = 128
	RNMPointStride       = 32
	RNMMaxPoints         = 4096
	RNMMaxSamples        = 64
	RNMCoefficientStride = 48
)

//go:embed wgsl/rnm.wgsl
var RNMSource string

// RNMScene controls a deterministic, progressive directional irradiance bake.
// Environment colors are irradiance units; environment radiance is color/pi.
// Zero Samples selects eight samples, zero RowsPerRecord selects eight rows.
// Bounces is zero or one. Geometry and mean linear reflectance are prepared offline.
// Prepared transport uses its build-time GISamples; Samples and Seed control only
// the v2 ray-traced fallback and never change prepared geometry transport.
type RNMScene struct {
	Points                                []Point
	SkyColor, GroundColor                 [3]float32
	Samples, Bounces, Seed, RowsPerRecord uint32
}

var errRNM = errors.New("lightmap: RNM requires prepared frames/surfaces and bounded finite settings")

// RNMParameters admits settings and packs the exact bytes used by both executors.
// Returned point bytes reserve the maximum admitted list, permitting editor changes
// without reallocating GPU resources. No geometry is transformed here.
func RNMParameters(document *lightmapdata.Document, scene RNMScene) ([RNMUniformSize]byte, []byte, float32, RNMScene, error) {
	var result [RNMUniformSize]byte
	if !validRNMDocument(document) || len(scene.Points) > RNMMaxPoints || scene.Bounces > 1 || scene.Samples > RNMMaxSamples {
		return result, nil, 0, scene, errRNM
	}

	if scene.Samples == 0 {
		scene.Samples = 8
	}

	if scene.RowsPerRecord == 0 {
		scene.RowsPerRecord = 8
	}

	scene.RowsPerRecord = min(scene.RowsPerRecord, document.Height)
	if len(document.ReceiverGIIndex) > 0 {
		scene.Samples = document.GISamples
	}

	points, energy, err := rnmPointBytes(document, scene.Points)
	if err != nil {
		return result, nil, 0, scene, err
	}

	environment, err := rnmEnvironmentBound(scene)
	if err != nil {
		return result, nil, 0, scene, err
	}
	// Uniform-hemisphere sampling has weight 2*pi. Reflectance is capped at .95.
	const (
		hemisphereBound  float32 = 2
		reflectanceBound float32 = .95
	)

	scale := energy + hemisphereBound*environment
	if scene.Bounces > 0 {
		scale += hemisphereBound * reflectanceBound * (energy + environment)
	}

	scale = max(float32(1), scale)
	if math.IsInf(float64(scale), 0) {
		return result, nil, 0, scene, errEnergy
	}

	packRNMUniforms(&result, document, scene, scale)

	return result, points, scale, scene, nil
}

func validRNMDocument(document *lightmapdata.Document) bool {
	return document != nil && len(document.Frames) == int(document.Width)*int(document.Height)*16 &&
		len(document.Surfaces) == len(document.Triangles)/lightmapdata.TriangleStride*16 &&
		len(document.GPUData) > 0 && len(document.GPUData) <= 128<<20 &&
		RNMScratchSize(document) <= 128<<20
}

func rnmPointBytes(document *lightmapdata.Document, sources []Point) ([]byte, float32, error) {
	var energy float32

	points := make([]byte, RNMMaxPoints*RNMPointStride)

	for index, point := range sources {
		if _, _, err := Parameters(document, point); err != nil {
			return nil, 0, err
		}

		if point.Bias == 0 {
			point.Bias = .015
		}

		values := []float32{point.Position[0], point.Position[1], point.Position[2], point.Range,
			point.Color[0] * point.Intensity, point.Color[1] * point.Intensity, point.Color[2] * point.Intensity, point.Bias}
		for component, value := range values {
			binary.LittleEndian.PutUint32(points[index*RNMPointStride+component*4:], math.Float32bits(value))
		}

		energy += point.Intensity * max(point.Color[0], point.Color[1], point.Color[2])
	}

	return points, energy, nil
}

func rnmEnvironmentBound(scene RNMScene) (float32, error) {
	var environment float32

	for _, color := range [][3]float32{scene.SkyColor, scene.GroundColor} {
		for _, value := range color {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < 0 || value > 1 {
				return 0, errRNM
			}

			environment = max(environment, value)
		}
	}

	return environment, nil
}

//nolint:gosec // Admitted immutable document lengths are bounded to 256 MiB and the light list to 4096 before packing.
func packRNMUniforms(result *[RNMUniformSize]byte, document *lightmapdata.Document, scene RNMScene, scale float32) {
	words := []uint32{
		document.Width,
		document.Height,
		uint32(len(document.Nodes) / lightmapdata.NodeStride),
		uint32(len(document.Triangles) / lightmapdata.TriangleStride),
		0,
		uint32(len(document.Nodes) / 4),
		uint32((len(document.Nodes) + len(document.Triangles)) / 4),
		uint32((len(document.Nodes) + len(document.Triangles) + len(document.Receivers)) / 4),
		uint32(
			(len(document.Nodes) + len(document.Triangles) + len(document.Receivers) + len(document.Padding)) / 4,
		),
		uint32(
			(len(document.Nodes) + len(document.Triangles) + len(document.Receivers) + len(document.Padding) + len(document.Frames)) / 4,
		),
		uint32(len(scene.Points)),
		scene.Samples,
		scene.Bounces,
		scene.Seed,
		0,
		scene.RowsPerRecord,
	}
	for index, value := range words {
		binary.LittleEndian.PutUint32(result[index*4:], value)
	}

	values := []float32{
		scene.SkyColor[0],
		scene.SkyColor[1],
		scene.SkyColor[2],
		scale,
		scene.GroundColor[0],
		scene.GroundColor[1],
		scene.GroundColor[2],
		.015,
	}
	for index, value := range values {
		binary.LittleEndian.PutUint32(result[64+index*4:], math.Float32bits(value))
	}

	prefix := len(document.Nodes) + len(document.Triangles) + len(document.Receivers)
	prefix += len(document.Padding) + len(document.Frames) + len(document.Surfaces)

	offsets := []uint32{
		uint32(prefix / 4),
		uint32((prefix + len(document.ReceiverGIIndex)) / 4),
		uint32((prefix + len(document.ReceiverGIIndex) + len(document.GIReceivers)) / 4),
		uint32((prefix + len(document.ReceiverGIIndex) + len(document.GIReceivers) + len(document.GIPatches)) / 4),
		uint32(
			len(document.GIReceivers) / 32,
		),
		uint32(len(document.GIPatches) / 48),
		document.Width * document.Height * 3,
		document.GISamples,
	}
	for index, value := range offsets {
		binary.LittleEndian.PutUint32(result[96+index*4:], value)
	}
}

// RNMRows changes only the row interval, preserving deterministic global sampling.
func RNMRows(parameters *[RNMUniformSize]byte, first, count uint32) {
	binary.LittleEndian.PutUint32(parameters[56:], first)
	binary.LittleEndian.PutUint32(parameters[60:], count)
}

// RNMDescription uses three storage buffers and three output images, within
// WebGPU compatibility limits. Both entrypoints share one explicit layout.
func RNMDescription(document *lightmapdata.Document, entry string) compute.PipelineDesc {
	return compute.PipelineDesc{Program: shader.WGSL("lightmap/wgsl/rnm.wgsl", RNMSource), EntryPoint: entry, Layout: shader.Layout{
		shader.UniformLayout(0, RNMUniformSize), shader.ReadOnlyStorageLayout(1, uint64(len(document.GPUData))),
		shader.ReadOnlyStorageLayout(2, RNMMaxPoints*RNMPointStride),
		shader.StorageLayout(3, RNMScratchSize(document)),
		shader.StorageTextureLayout(4), shader.StorageTextureLayout(5), shader.StorageTextureLayout(6),
	}}
}

// RNMScratchSize includes three receiver coefficient vectors and one outgoing
// reflected irradiance vector per prepared patch, sharing a single storage buffer.
func RNMScratchSize(document *lightmapdata.Document) uint64 {
	return uint64(document.Width)*uint64(document.Height)*RNMCoefficientStride + uint64(len(document.GIPatches)/48)*16
}
