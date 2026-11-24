//go:build linux

package lightmapbuild_test

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
	"github.com/karty-game/ashura/internal/lightmapkernel"
	"github.com/karty-game/ashura/pkg/lightmap"
	lightmapbuild "github.com/karty-game/ashura/pkg/lightmap/build"
	"github.com/karty-game/ashura/pkg/texture"
)

// transportFixture independently integrates the tiny fixture's one analytic
// triangle with the fallback's published deterministic primary sequence. Uniform
// environment makes its red diffuse outgoing radiance identical at every hit,
// allowing an analytic prepared-vs-ray-traced one-bounce comparison.
func transportFixture(t *testing.T, prepared bool) *lightmap.Asset {
	t.Helper()

	document := lightmapfixture.RNMDocument()
	putTransportFloat(document.Surfaces, 0, .95, 0, 0, 0)

	if prepared {
		document.GISamples = 64

		document.ReceiverGIIndex = make([]byte, 16)
		for index, value := range []uint32{0, 1, math.MaxUint32, math.MaxUint32} {
			binary.LittleEndian.PutUint32(document.ReceiverGIIndex[index*4:], value)
		}

		document.GIReceivers = make([]byte, 64)
		document.GIPatches = make([]byte, 48)
		document.GITransfers = make([]byte, 32)
		putTransportFloat(document.GIPatches, 0, -.5, 1, 0, .5, 0, -1, 0, .5, .95, 0, 0, 0)

		bases := [3][3]float32{
			{.816496580927726, .577350269189626, 0},
			{-.408248290463863, .577350269189626, -.707106781186548},
			{-.408248290463863, .577350269189626, .707106781186548},
		}

		for receiver := range 2 {
			var sky, ground, transfer [3]float32

			receiverX := float32(receiver*2 - 1)
			rotation := transportRandom(transportHash(uint32(receiver)))

			for sample := range 64 {
				upward := (float32(sample) + .5) / 64
				v := float32(sample)*.618033988749895 + rotation
				v -= float32(math.Floor(float64(v)))
				radius := float32(math.Sqrt(float64(1 - upward*upward)))
				angle := float32(2*math.Pi) * v
				direction := [3]float32{radius * float32(math.Sin(float64(angle))), upward, radius * float32(math.Cos(float64(angle)))}
				distance := (1 - float32(.015)) / upward
				x, z := receiverX+direction[0]*distance, direction[2]*distance
				hit := x >= -2 && x <= 0 && z >= -2 && z <= 2*x+2

				for basis := range 3 {
					cosine := max(float32(0), bases[basis][0]*direction[0]+bases[basis][1]*direction[1]+bases[basis][2]*direction[2])

					weight := 2 * cosine / 64
					if hit {
						transfer[basis] += weight
					} else {
						sky[basis] += weight * (.5 + .5*upward)
						ground[basis] += weight * (.5 - .5*upward)
					}
				}
			}

			binary.LittleEndian.PutUint32(document.GIReceivers[receiver*32:], uint32(receiver))
			binary.LittleEndian.PutUint32(document.GIReceivers[receiver*32+4:], 1)
			putTransportFloat(document.GIReceivers, receiver*32+8, sky[0], sky[1], sky[2], ground[0], ground[1], ground[2])
			putTransportFloat(document.GITransfers, receiver*16+4, transfer[0], transfer[1], transfer[2])
		}
	}

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	return asset
}

func putTransportFloat(destination []byte, offset int, values ...float32) {
	for index, value := range values {
		binary.LittleEndian.PutUint32(destination[offset+index*4:], math.Float32bits(value))
	}
}
func transportHash(value uint32) uint32 {
	value = (value ^ (value >> 16)) * 0x7feb352d
	value = (value ^ (value >> 15)) * 0x846ca68b

	return value ^ (value >> 16)
}
func transportRandom(value uint32) float32 {
	return float32(transportHash(value)&0x00ffffff) / 16777216
}

func TestPreparedTransportFixtureAdmissionAndScratch(t *testing.T) {
	t.Parallel()
	asset := transportFixture(t, true)

	parameters, _, _, normalized, err := lightmapkernel.RNMParameters(asset.Document(), lightmap.RNMScene{Samples: 8, Bounces: 1})
	if err != nil {
		t.Fatal(err)
	}

	if normalized.Samples != 64 || binary.LittleEndian.Uint32(parameters[112:]) != 2 || binary.LittleEndian.Uint32(parameters[116:]) != 1 ||
		binary.LittleEndian.Uint32(parameters[120:]) != 12 {
		t.Fatal("prepared uniform offsets/counts or effective quality incorrect")
	}

	if lightmapkernel.RNMScratchSize(asset.Document()) != 4*48+16 {
		t.Fatal("patch scratch does not follow receiver coefficients")
	}

	for _, entry := range []string{"bake_rnm_patches", "bake_rnm", "resolve_rnm"} {
		desc := lightmapkernel.RNMDescription(asset.Document(), entry)
		if desc.Layout[0].MinSize != 128 || desc.Layout[3].MinSize != 4*48+16 {
			t.Fatal("shared prepared compute layout incorrect")
		}
	}
}

func TestHardwarePreparedTransportAndRGBE(t *testing.T) {
	t.Parallel()

	if os.Getenv("KARTY_LIGHTMAP_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAP_GPU_TEST=1 with hardware GPU access")
	}

	traced, prepared := transportFixture(t, false), transportFixture(t, true)
	bake := func(asset *lightmap.Asset, scene lightmap.RNMScene) lightmapbuild.RNMResult {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := lightmapbuild.BakeRNMBrowser(
			ctx,
			asset,
			scene,
			lightmapbuild.BrowserOptions{Executable: os.Getenv("KARTY_LIGHTMAP_BROWSER")},
		)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf(
			"hardware=%s duration=%s scale=%g samples=%d bounces=%d encoding=%d",
			result.Adapter,
			result.Duration,
			result.Scale,
			result.Samples,
			result.Bounces,
			result.Encoding,
		)

		return result
	}
	pointResult := bake(
		prepared,
		lightmap.RNMScene{Points: []lightmap.Point{{Position: [3]float32{0, 2, 0}, Color: [3]float32{1, .5, 0}, Intensity: 2, Range: 10}}},
	)
	expectedCosines := [3]float64{
		(-math.Sqrt(2.0/3) + 2/math.Sqrt(3)) / math.Sqrt(5),
		(1/math.Sqrt(6) + 2/math.Sqrt(3)) / math.Sqrt(5),
		(1/math.Sqrt(6) + 2/math.Sqrt(3)) / math.Sqrt(5),
	}

	for basis := range 3 {
		blocked := decodeTransportPixel(pointResult, basis, 0)
		lit := decodeTransportPixel(pointResult, basis, 1)

		expected := 2 * .95 * .95 * expectedCosines[basis]
		if blocked != ([3]float64{}) || math.Abs(lit[0]-expected) > expected/255+1e-5 || math.Abs(lit[1]-expected*.5) > expected/255+1e-5 ||
			lit[2] != 0 {
			t.Fatalf("prepared direct visibility/basis failed: basis%d blocked%v lit%v expected%g", basis, blocked, lit, expected)
		}
	}

	scene := lightmap.RNMScene{SkyColor: [3]float32{.2, .2, .2}, GroundColor: [3]float32{.2, .2, .2}, Samples: 64, Bounces: 1}
	directScene := scene
	directScene.Bounces = 0
	direct := bake(prepared, directScene)
	rayResult := bake(traced, scene)

	result := bake(prepared, scene)
	if result.Encoding != texture.RNMRGBE || result.Samples != 64 || rayResult.Encoding != texture.RNMLinear {
		t.Fatal("encoding or prepared sample metadata incorrect")
	}

	var redGain float64

	for basis := range 3 {
		for receiver := range 2 {
			actual := decodeTransportPixel(result, basis, receiver)
			expected := decodeTransportPixel(rayResult, basis, receiver)
			old := decodeTransportPixel(direct, basis, receiver)

			for channel := range 3 {
				// Linear fallback has half an absolute RGBA8 step of error;
				// RGBE has half a relative mantissa step plus float arithmetic.
				tolerance := float64(rayResult.Scale)/510 + math.Max(actual[0], math.Max(actual[1], actual[2]))/255 + 1e-5
				if math.Abs(actual[channel]-expected[channel]) > tolerance {
					t.Fatalf(
						"prepared vs RT mismatch: basis%d pixel%d channel%d got%g want%g tolerance%g",
						basis,
						receiver,
						channel,
						actual[channel],
						expected[channel],
						tolerance,
					)
				}

				if channel > 0 && math.Abs(actual[channel]-old[channel]) > tolerance {
					t.Fatal("red patch introduced non-red indirect energy")
				}
			}

			redGain += actual[0] - old[0]
		}
	}

	if redGain < .01 {
		t.Fatalf("prepared diffuse color bleed missing: %g", redGain)
	}

	scene = lightmap.RNMScene{SkyColor: [3]float32{.0001, .0001, .0001}, GroundColor: [3]float32{.0001, .0001, .0001}, Samples: 8}

	dim := bake(prepared, scene)
	for basis := range 3 {
		pixel := decodeTransportPixel(dim, basis, 1)
		if pixel[0] <= 0 || pixel[0] > .0002 {
			t.Fatalf("RGBE lost weak illumination or saturated: %g", pixel[0])
		}
	}
}

func decodeTransportPixel(result lightmapbuild.RNMResult, basis, pixel int) [3]float64 {
	data := result.Textures[basis].Levels[0][pixel*4:]

	multiplier := float64(result.Scale) / 255
	if result.Encoding == texture.RNMRGBE {
		if data[3] == 0 {
			return [3]float64{}
		}

		multiplier *= math.Exp2(float64(data[3]) - 128)
	}

	return [3]float64{float64(data[0]) * multiplier, float64(data[1]) * multiplier, float64(data[2]) * multiplier}
}
