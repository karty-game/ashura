// Package lightmapkernel owns the identical offline and runtime bake program.
package lightmapkernel

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"math"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/shader"
)

const UniformSize = 64
const WorkgroupSize = 8

//go:embed wgsl/direct.wgsl
var Source string

var (
	errPoint  = errors.New("lightmap: invalid point light")
	errFinite = errors.New("lightmap: nonfinite point light")
	errColor  = errors.New("lightmap: point color is outside [0,1]")
	errBias   = errors.New("lightmap: bias exceeds light range")
	errEnergy = errors.New("lightmap: light energy exceeds numeric range")
)

// Point describes one direct source in prepared-model coordinates.
type Point struct {
	Position               [3]float32
	Color                  [3]float32
	Intensity, Range, Bias float32
}

// Parameters validates all settings and returns explicitly packed WGSL bytes.
// Zero bias selects .015 world units, matching the live point-shadow default.
//
//nolint:gosec // Admitted counts are bounded before uint32 packing.
func Parameters(document *lightmapdata.Document, point Point) ([UniformSize]byte, float32, error) {
	var result [UniformSize]byte
	if document == nil || point.Range <= 0 || point.Intensity < 0 || point.Bias < 0 {
		return result, 0, errPoint
	}

	for _, v := range []float32{point.Position[0], point.Position[1], point.Position[2], point.Color[0], point.Color[1], point.Color[2], point.Intensity, point.Range, point.Bias} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return result, 0, errFinite
		}
	}

	for _, v := range point.Color {
		if v < 0 || v > 1 {
			return result, 0, errColor
		}
	}

	if point.Bias == 0 {
		point.Bias = .015
	}

	if point.Bias >= point.Range {
		return result, 0, errBias
	}

	scale := max(float32(1), point.Intensity*max(point.Color[0], point.Color[1], point.Color[2]))
	if math.IsInf(float64(scale), 0) {
		return result, 0, errEnergy
	}

	values := []float32{
		point.Position[0],
		point.Position[1],
		point.Position[2],
		point.Range,
		point.Color[0] * point.Intensity,
		point.Color[1] * point.Intensity,
		point.Color[2] * point.Intensity,
		scale,
	}
	for i, v := range values {
		binary.LittleEndian.PutUint32(result[i*4:], math.Float32bits(v))
	}

	for i, v := range []uint32{document.Width, document.Height, uint32(len(document.Nodes) / lightmapdata.NodeStride), uint32(len(document.Triangles) / lightmapdata.TriangleStride)} {
		binary.LittleEndian.PutUint32(result[32+i*4:], v)
	}

	binary.LittleEndian.PutUint32(result[48:], math.Float32bits(point.Bias))

	return result, scale, nil
}

func Description(document *lightmapdata.Document) compute.PipelineDesc {
	return compute.PipelineDesc{Program: shader.WGSL("lightmap/wgsl/direct.wgsl", Source), EntryPoint: "bake_direct", Layout: shader.Layout{
		shader.UniformLayout(0, UniformSize),
		shader.ReadOnlyStorageLayout(1, uint64(len(document.Nodes))),
		shader.ReadOnlyStorageLayout(2, uint64(len(document.Triangles))),
		shader.ReadOnlyStorageLayout(3, uint64(len(document.Receivers))),
		shader.ReadOnlyStorageLayout(4, uint64(len(document.Padding))),
		shader.StorageTextureLayout(5),
	}}
}
