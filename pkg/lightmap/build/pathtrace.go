//go:build !js

package lightmapbuild

import (
	"context"
	"encoding/binary"
	"errors"
	"math"

	"github.com/karty-game/ashura/internal/lightmapkernel"
	"github.com/karty-game/ashura/pkg/lightmap"
)

// PathTraceOptions controls only offline production tracing. Samples defaults
// to256 and is bounded to4096; Bounces is the number of diffuse reflecting
// surfaces after the receiver (zero through eight). Seed is deterministic.
type PathTraceOptions struct {
	Samples, Bounces, Seed uint32
}

var errPathTrace = errors.New("lightmap: offline path tracing requires bounded settings and prepared geometric normals")

// BakePathTraced computes physical linear RNM irradiance through the complete
// visibility scene, bypassing the coarse prepared patches. Its three maps use
// RGBE with Scale1; the exact runtime/editor Bake API remains unchanged.
// geometricNormals is the matching preparation Result.GeometricNormals buffer.
func (session *RNMSession) BakePathTraced(
	ctx context.Context,
	asset *lightmap.Asset,
	geometricNormals []byte,
	scene lightmap.RNMScene,
	options PathTraceOptions,
) (RNMResult, error) {
	return session.bake(ctx, asset, scene, geometricNormals, &options)
}

func pathTraceParameters(
	asset *lightmap.Asset,
	geometricNormals []byte,
	scene lightmap.RNMScene,
	options PathTraceOptions,
) ([lightmapkernel.RNMUniformSize]byte, []byte, float32, lightmap.RNMScene, error) {
	var empty [lightmapkernel.RNMUniformSize]byte
	if asset == nil || options.Samples > 4096 || options.Bounces > 8 {
		return empty, nil, 0, scene, errPathTrace
	}

	if options.Samples == 0 {
		options.Samples = 256
	}

	document := asset.Document()
	if !validPathTraceNormals(geometricNormals, document.Receivers, int(document.Width)*int(document.Height)) {
		return empty, nil, 0, scene, errPathTrace
	}
	// Reuse shared admission for lights, scene bounds, and GPU ABI without
	// broadening runtime RNM's sample/bounce limits. Production overrides only
	// its own shader controls after successful admission.
	scene.Samples = 1
	scene.Bounces = 0

	parameters, points, _, normalized, err := lightmapkernel.RNMParameters(document, scene)
	if err != nil {
		return empty, nil, 0, scene, err
	}

	if !validPathTraceEnergy(scene, options.Bounces) {
		return empty, nil, 0, scene, errPathTrace
	}

	set := func(offset int, value uint32) { binary.LittleEndian.PutUint32(parameters[offset:], value) }
	set(44, options.Samples)
	set(48, options.Bounces)
	set(52, options.Seed)
	set(76, math.Float32bits(1))
	// Production tracing never allocates or dispatches coarse patch scratch.
	set(112, 0)
	set(116, 0)

	normalized.Samples, normalized.Bounces, normalized.Seed = options.Samples, options.Bounces, options.Seed

	return parameters, points, 1, normalized, nil
}

func validPathTraceNormals(normals, receivers []byte, pixels int) bool {
	if len(normals) != pixels*16 {
		return false
	}

	for pixel := range pixels {
		var squared float64

		for axis := range 3 {
			value := math.Float32frombits(binary.LittleEndian.Uint32(normals[pixel*16+axis*4:]))
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return false
			}

			squared += float64(value) * float64(value)
		}

		coverage := math.Float32frombits(binary.LittleEndian.Uint32(receivers[pixel*32+12:]))
		if coverage >= .5 && math.Abs(squared-1) > .001 {
			return false
		}
	}

	return true
}

// The primary uniform-hemisphere estimator can contribute at most twice the
// incoming irradiance bound. Each cosine-sampled hit attenuates it by the
// admitted maximum reflectance .95. Check in float64 before physical Scale1
// accumulation; normalized legacy RNM can represent larger authored energies.
//
//nolint:mnd // RNM integration and RGBE exponent limits.
func validPathTraceEnergy(scene lightmap.RNMScene, bounces uint32) bool {
	var energy, environment float64
	for _, point := range scene.Points {
		energy += float64(point.Intensity) * float64(max(point.Color[0], point.Color[1], point.Color[2]))
	}

	for _, color := range [][3]float32{scene.SkyColor, scene.GroundColor} {
		for _, channel := range color {
			environment = max(environment, float64(channel))
		}
	}

	factor, throughput := 1.0, 1.0
	for range bounces {
		throughput *= .95
		factor += 2 * throughput
	}

	return energy*factor+2*environment <= math.Exp2(127)
}
