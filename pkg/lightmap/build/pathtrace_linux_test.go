//go:build linux

package lightmapbuild_test

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	lightmapbuild "github.com/karty-game/karty-webgpu/pkg/lightmap/build"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestHardwarePathTracedRNMAnalyticEnvironmentAndMultipleBounces(t *testing.T) {
	t.Parallel()

	if os.Getenv("KARTY_LIGHTMAP_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAP_GPU_TEST=1 with hardware GPU access")
	}

	session, err := lightmapbuild.NewRNMSession(t.Context(), lightmapbuild.BrowserOptions{Executable: os.Getenv("KARTY_LIGHTMAP_BROWSER")})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	normals := make([]byte, 4*16)
	for pixel := range 2 {
		putTransportFloat(normals, pixel*16, 0, 1, 0)
	}

	bake := func(document lightmapdata.Document, scene lightmap.RNMScene, bounces uint32) lightmapbuild.RNMResult {
		t.Helper()

		encoded, encodeErr := lightmapdata.Encode(document)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}

		asset, decodeErr := lightmap.Decode(encoded)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}

		result, bakeErr := session.BakePathTraced(
			t.Context(),
			asset,
			normals,
			scene,
			lightmapbuild.PathTraceOptions{Samples: 1024, Bounces: bounces, Seed: 29},
		)
		if bakeErr != nil {
			t.Fatal(bakeErr)
		}

		if result.Scale != 1 || result.Encoding != texture.RNMRGBE || result.Bounces != bounces || result.Samples != 1024 {
			t.Fatal("production result metadata lost", result)
		}

		t.Logf("hardware=%s duration=%s samples=%d bounces=%d", result.Adapter, result.Duration, result.Samples, result.Bounces)

		return result
	}
	checkPathTraceEnvironment(t, bake)
	checkPathTraceMultipleBounces(t, bake)
}

func checkPathTraceEnvironment(t *testing.T, bake func(lightmapdata.Document, lightmap.RNMScene, uint32) lightmapbuild.RNMResult) {
	t.Helper()

	// Move the only occluder below the receiving hemisphere. Uniform environment
	// has analytic clipped-cosine RNM integral (1+dot(basis,normal))/2.
	open := lightmapfixture.RNMDocument()
	putTransportFloat(open.Nodes, 0, -2, -1, -2)
	putTransportFloat(open.Nodes, 16, 0, -1, 2)

	for vertex := range 3 {
		putTransportFloat(open.Triangles, vertex*16+4, -1)
	}

	environment := float64(.2)
	result := bake(open, lightmap.RNMScene{SkyColor: [3]float32{.2, .2, .2}, GroundColor: [3]float32{.2, .2, .2}}, 4)
	expected := environment * (1 + 1/math.Sqrt(3)) / 2

	for basis := range 3 {
		for pixel := range 2 {
			actual := decodeTransportPixel(result, basis, pixel)
			for _, value := range actual {
				if math.Abs(value-expected) > .002 {
					t.Fatalf("environment normalization basis%d pixel%d got%v want%g", basis, pixel, actual, expected)
				}
			}
		}

		if decodeTransportPixel(result, basis, 2) != decodeTransportPixel(result, basis, 1) ||
			decodeTransportPixel(result, basis, 3) != ([3]float64{}) {
			t.Fatal("production resolve lost chart gutters or empty texels")
		}
	}
}

func checkPathTraceMultipleBounces(t *testing.T, bake func(lightmapdata.Document, lightmap.RNMScene, uint32) lightmapbuild.RNMResult) {
	t.Helper()

	// Two large facing diffuse surfaces admit paths with two or more reflections.
	// A red ceiling ensures extra transport cannot introduce green/blue energy.
	closed := lightmapfixture.RNMDocument()
	closed.Triangles = make([]byte, 2*lightmapdata.TriangleStride)
	closed.Surfaces = make([]byte, 2*lightmapdata.SurfaceStride)
	putTransportFloat(closed.Nodes, 0, -100, 0, -100)
	putTransportFloat(closed.Nodes, 16, 100, 1, 100)
	binary.LittleEndian.PutUint32(closed.Nodes[28:], 2)
	putTransportFloat(closed.Triangles, 0, -100, 1, -100)
	putTransportFloat(closed.Triangles, 16, 100, 1, -100)
	putTransportFloat(closed.Triangles, 32, 0, 1, 100)
	putTransportFloat(closed.Triangles, 48, -100, 0, -100)
	putTransportFloat(closed.Triangles, 64, 0, 0, 100)
	putTransportFloat(closed.Triangles, 80, 100, 0, -100)
	putTransportFloat(closed.Surfaces, 0, .8, 0, 0, 0)
	putTransportFloat(closed.Surfaces, 16, .8, .8, .8, 0)

	scene := lightmap.RNMScene{
		Points: []lightmap.Point{{Position: [3]float32{0, .5, 0}, Color: [3]float32{1, 1, 1}, Intensity: 1, Range: 10}},
	}
	direct, once, multiple := bake(closed, scene, 0), bake(closed, scene, 1), bake(closed, scene, 4)

	var firstGain, laterGain float64

	for basis := range 3 {
		for pixel := range 2 {
			before := decodeTransportPixel(direct, basis, pixel)
			one := decodeTransportPixel(once, basis, pixel)
			many := decodeTransportPixel(multiple, basis, pixel)
			firstGain += one[0] - before[0]

			laterGain += many[0] - one[0]
			for channel := 1; channel < 3; channel++ {
				if math.Abs(many[channel]-before[channel]) > .006 {
					t.Fatal("red diffuse bounce leaked energy into another channel", before, many)
				}
			}
		}
	}

	if firstGain < .01 || laterGain < .01 {
		t.Fatalf("missing first or subsequent diffuse bounce: first%g later%g", firstGain, laterGain)
	}
}
