//go:build !js

package lightmapbuild

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
	"github.com/karty-game/ashura/internal/lightmapkernel"
	"github.com/karty-game/ashura/pkg/lightmap"
)

func pathTraceFixture(t *testing.T) (*lightmap.Asset, []byte) {
	t.Helper()

	encoded, err := lightmapdata.Encode(lightmapfixture.TransportDocument())
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	normals := make([]byte, 4*16)
	for pixel := range 2 {
		binary.LittleEndian.PutUint32(normals[pixel*16+4:], math.Float32bits(1))
	}

	return asset, normals
}

func TestPathTraceParametersPreservePhysicalScaleAndRuntimeBounds(t *testing.T) {
	t.Parallel()
	asset, normals := pathTraceFixture(t)
	scene := lightmap.RNMScene{SkyColor: [3]float32{.2, .3, .4}, RowsPerRecord: 1}
	options := PathTraceOptions{Samples: 256, Bounces: 4, Seed: 17}

	parameters, points, scale, normalized, err := pathTraceParameters(asset, normals, scene, options)
	if err != nil {
		t.Fatal(err)
	}

	if scale != 1 || normalized.Samples != 256 || normalized.Bounces != 4 || normalized.Seed != 17 ||
		binary.LittleEndian.Uint32(parameters[44:]) != 256 || binary.LittleEndian.Uint32(parameters[48:]) != 4 ||
		binary.LittleEndian.Uint32(parameters[52:]) != 17 || math.Float32frombits(binary.LittleEndian.Uint32(parameters[76:])) != 1 {
		t.Fatal("production quality or physical scale lost")
	}

	if binary.LittleEndian.Uint32(parameters[112:]) != 0 || binary.LittleEndian.Uint32(parameters[116:]) != 0 ||
		len(points) != lightmapkernel.RNMMaxPoints*lightmapkernel.RNMPointStride {
		t.Fatal("production retained coarse transport or dropped admitted points")
	}

	if _, _, _, _, err := lightmapkernel.RNMParameters(asset.Document(), lightmap.RNMScene{Bounces: 4}); err == nil {
		t.Fatal("offline quality broadened runtime admission")
	}

	parameters, _, _, normalized, err = pathTraceParameters(asset, normals, scene, PathTraceOptions{})
	if err != nil || normalized.Samples != 256 || normalized.Bounces != 0 || binary.LittleEndian.Uint32(parameters[48:]) != 0 {
		t.Fatal("default samples or explicit zero-bounce reference lost", err)
	}
}

func TestPathTraceRejectsMissingOrInvalidGeometricNormals(t *testing.T) {
	t.Parallel()

	asset, normals := pathTraceFixture(t)
	for _, test := range []struct {
		name    string
		normals []byte
		options PathTraceOptions
	}{
		{name: "missing"},
		{name: "short", normals: normals[:len(normals)-1]},
		{name: "zero covered normal", normals: make([]byte, len(normals))},
		{name: "samples", normals: normals, options: PathTraceOptions{Samples: 4097}},
		{name: "bounces", normals: normals, options: PathTraceOptions{Bounces: 9}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, _, _, _, err := pathTraceParameters(
				asset,
				test.normals,
				lightmap.RNMScene{},
				test.options,
			); !errors.Is(
				err,
				errPathTrace,
			) {
				t.Fatal("accepted invalid production input", err)
			}
		})
	}

	for _, value := range []float32{2, float32(math.NaN()), float32(math.Inf(1))} {
		invalid := append([]byte(nil), normals...)
		binary.LittleEndian.PutUint32(invalid[4:], math.Float32bits(value))

		if _, _, _, _, err := pathTraceParameters(asset, invalid, lightmap.RNMScene{}, PathTraceOptions{}); !errors.Is(err, errPathTrace) {
			t.Fatal("accepted non-unit or non-finite geometric normal", value, err)
		}
	}
}

func TestPathTraceRejectsPhysicalRGBEOverflowWithoutChangingLegacyAdmission(t *testing.T) {
	t.Parallel()
	asset, normals := pathTraceFixture(t)

	scene := lightmap.RNMScene{Points: []lightmap.Point{{Range: 10, Color: [3]float32{1, 1, 1}, Intensity: math.MaxFloat32}}}
	if _, _, _, _, err := lightmapkernel.RNMParameters(asset.Document(), scene); err != nil {
		t.Fatal("fixture should remain valid for normalized legacy RNM", err)
	}

	if _, _, _, _, err := pathTraceParameters(asset, normals, scene, PathTraceOptions{Bounces: 4}); !errors.Is(err, errPathTrace) {
		t.Fatal("unrepresentable physical irradiance admitted", err)
	}
}
