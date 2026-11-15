//go:build !js

package lightmapbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestDenoiseConstantAndNoisyIndirect(t *testing.T) {
	t.Parallel()
	asset := denoiseTestAsset(t, 9, 9, nil)
	direct := denoiseTestResult(t, asset, 1, func(int) float64 { return .5 })
	constant := denoiseTestResult(t, asset, 1, func(int) float64 { return .875 })

	filtered, err := DenoiseRNM(t.Context(), asset, constant, direct, DenoiseOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for basis := range 3 {
		if !bytes.Equal(filtered.Textures[basis].Levels[0], constant.Textures[basis].Levels[0]) {
			t.Fatal("constant irradiance changed")
		}
	}

	noisy := denoiseTestResult(t, asset, 1, func(pixel int) float64 {
		if (pixel/9+pixel%9)%2 == 0 {
			return .7
		}

		return 1.3
	})
	before := append([]byte(nil), noisy.Textures[0].Levels[0]...)

	filtered, err = DenoiseRNM(t.Context(), asset, noisy, direct, DenoiseOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if denoiseTestVariance(filtered) >= denoiseTestVariance(noisy)*.4 {
		t.Fatal("indirect variance was not reduced")
	}

	if !bytes.Equal(noisy.Textures[0].Levels[0], before) {
		t.Fatal("input irradiance was mutated")
	}

	first := denoiseDecode(filtered.Textures[0].Levels[0][40*4:], filtered.Scale)

	third := denoiseDecode(filtered.Textures[2].Levels[0][40*4:], filtered.Scale)
	if math.Abs(first[1]/first[0]-.5) > .02 || math.Abs(third[0]/first[0]-3) > .05 {
		t.Fatal("RNM lobes/channels used inconsistent filter weights")
	}
}

func TestDenoiseKeepsChartGeometryAndTangentBoundaries(t *testing.T) {
	t.Parallel()

	for _, boundary := range []string{"chart", "normal", "tangent", "handedness", "parallel-wall", "occluding-wall"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			asset := denoiseTestAsset(t, 8, 3, func(document *lightmapdata.Document) {
				denoiseTestBoundary(document, boundary)
			})
			direct := denoiseTestResult(t, asset, 1, func(int) float64 { return 0 })
			total := denoiseTestResult(t, asset, 1, func(pixel int) float64 {
				if pixel%8 < 4 {
					return .25
				}

				return .75
			})

			filtered, err := DenoiseRNM(t.Context(), asset, total, direct, DenoiseOptions{})
			if err != nil {
				t.Fatal(err)
			}

			for basis := range 3 {
				if !bytes.Equal(filtered.Textures[basis].Levels[0], total.Textures[basis].Levels[0]) {
					t.Fatal("irradiance leaked across " + boundary)
				}
			}
		})
	}
}

func TestDenoisePreservesDirectShadowsAcrossIndependentScales(t *testing.T) {
	t.Parallel()
	asset := denoiseTestAsset(t, 8, 3, nil)
	direct := denoiseTestResult(t, asset, 2, func(pixel int) float64 {
		if pixel%8 < 4 {
			return 0
		}

		return 1
	})
	total := denoiseTestResult(t, asset, 8, func(pixel int) float64 {
		if pixel%8 < 4 {
			return .25
		}

		return 1.25
	})

	filtered, err := DenoiseRNM(t.Context(), asset, total, direct, DenoiseOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for pixel := range 24 {
		value := denoiseDecode(filtered.Textures[0].Levels[0][pixel*4:], filtered.Scale)

		point := denoiseDecode(direct.Textures[0].Levels[0][pixel*4:], direct.Scale)
		if math.Abs(value[0]-point[0]-.25) > .015 {
			t.Fatal("direct edge was filtered or scale was applied incorrectly")
		}
	}
}

func TestDenoiseRegeneratesGuttersAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	asset := denoiseTestAsset(t, 5, 1, func(document *lightmapdata.Document) {
		clear(document.Receivers[3*lightmapdata.ReceiverStride:])
		clear(document.Frames[3*lightmapdata.FrameStride:])
		binary.LittleEndian.PutUint32(document.Padding[12:], 1)
		binary.LittleEndian.PutUint32(document.Padding[16:], math.MaxUint32)
	})
	total := denoiseTestResult(t, asset, 1, func(pixel int) float64 { return .25 + float64(pixel)*.125 })
	direct := denoiseTestResult(t, asset, 1, func(int) float64 { return .125 })

	filtered, err := DenoiseRNM(t.Context(), asset, total, direct, DenoiseOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, image := range filtered.Textures {
		if !bytes.Equal(image.Levels[0][12:16], image.Levels[0][4:8]) || !bytes.Equal(image.Levels[0][16:20], []byte{0, 0, 0, 0}) {
			t.Fatal("gutter or empty output was not regenerated")
		}
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := DenoiseRNM(canceled, asset, total, direct, DenoiseOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal("pre-cancellation ignored", err)
	}

	large := denoiseTestAsset(t, 9, 9, nil)
	all := denoiseTestResult(t, large, 1, func(int) float64 { return .5 })

	stop := &denoiseCancelContext{done: make(chan struct{}), limit: 20}
	if _, err := DenoiseRNM(stop, large, all, all, DenoiseOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal("in-progress cancellation ignored", err)
	}
}

func TestDenoiseRejectsUnsupportedInputs(t *testing.T) {
	t.Parallel()
	asset := denoiseTestAsset(t, 4, 1, nil)

	all := denoiseTestResult(t, asset, 1, func(int) float64 { return .5 })
	for _, options := range []DenoiseOptions{{Radius: -1}, {Radius: 5}, {Passes: 5}, {Strength: 2}, {Strength: float32(math.NaN())}} {
		if _, err := DenoiseRNM(t.Context(), asset, all, all, options); err == nil {
			t.Fatal("invalid options accepted", options)
		}
	}

	invalid := all

	invalid.Scale = 0
	if _, err := DenoiseRNM(t.Context(), asset, invalid, all, DenoiseOptions{}); err == nil {
		t.Fatal("invalid scale accepted")
	}

	invalid = all

	invalid.Textures[0].Format = texture.BC1
	if _, err := DenoiseRNM(t.Context(), asset, invalid, all, DenoiseOptions{}); err == nil {
		t.Fatal("compressed data accepted as RGBE")
	}
}

func denoiseTestAsset(t *testing.T, width, height uint32, change func(*lightmapdata.Document)) *lightmap.Asset {
	t.Helper()

	if width < 1 || width > 2048 || height < 1 || height > 2048 {
		t.Fatal("fixture dimensions exceed prepared RNM bounds")
	}

	document := lightmapfixture.RNMDocument()
	document.Width, document.Height = width, height
	document.Receivers = make([]byte, width*height*lightmapdata.ReceiverStride)
	document.Frames = make([]byte, width*height*lightmapdata.FrameStride)

	document.Padding = make([]byte, width*height*4)
	for pixel := range width * height {
		denoiseTestPut(document.Receivers[pixel*32:], float32(pixel%width), 0, float32(pixel/width), 1, 0, 1, 0, 1)
		denoiseTestPut(document.Frames[pixel*16:], 1, 0, 0, 1)
		binary.LittleEndian.PutUint32(document.Padding[pixel*4:], pixel)
	}

	if change != nil {
		change(&document)
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

func denoiseTestResult(t *testing.T, asset *lightmap.Asset, scale float32, value func(int) float64) RNMResult {
	t.Helper()

	width, height := asset.Size()
	result := RNMResult{Scale: scale, Encoding: texture.RNMRGBE, Samples: 64}

	for basis := range 3 {
		image := texture.Data{Width: int(width), Height: int(height), ColorSpace: texture.Linear,
			Levels: [][]byte{make([]byte, int(width)*int(height)*4)}}
		for pixel := range int(width) * int(height) {
			red := value(pixel) * float64(basis+1)
			if err := denoiseEncode(image.Levels[0][pixel*4:], [3]float64{red, red * .5, red * .25}, scale); err != nil {
				t.Fatal(err)
			}
		}

		result.Textures[basis] = image
	}

	return result
}

func denoiseTestVariance(result RNMResult) float64 {
	pixels := result.Textures[0].Levels[0]

	var sum, squared float64

	for offset := 0; offset < len(pixels); offset += 4 {
		value := denoiseDecode(pixels[offset:], result.Scale)[0]
		sum, squared = sum+value, squared+value*value
	}

	count := float64(len(pixels) / 4)

	return squared/count - sum*sum/(count*count)
}

func denoiseTestPut(data []byte, values ...float32) {
	for index, value := range values {
		binary.LittleEndian.PutUint32(data[index*4:], math.Float32bits(value))
	}
}

func denoiseTestBoundary(document *lightmapdata.Document, boundary string) {
	if boundary == "occluding-wall" {
		denoiseTestWall(document)

		return
	}

	for pixel := range int(document.Width) * int(document.Height) {
		if pixel%int(document.Width) < 4 {
			continue
		}

		receiver, frame := document.Receivers[pixel*32:], document.Frames[pixel*16:]

		switch boundary {
		case "chart":
			denoiseTestPut(receiver[28:], 2)
		case "normal":
			denoiseTestPut(receiver[16:], 0, -1, 0)
		case "tangent":
			denoiseTestPut(frame, 0, 0, 1, 1)
		case "handedness":
			denoiseTestPut(frame[12:], -1)
		case "parallel-wall":
			denoiseTestPut(receiver[4:], 4)
		}
	}
}

func denoiseTestWall(document *lightmapdata.Document) {
	document.Triangles = make([]byte, 2*lightmapdata.TriangleStride)
	document.Surfaces = make([]byte, 2*lightmapdata.SurfaceStride)
	denoiseTestPut(document.Nodes, 3.5, -2, -2)
	denoiseTestPut(document.Nodes[16:], 3.5, 2, 10)
	binary.LittleEndian.PutUint32(document.Nodes[28:], 2)
	denoiseTestPut(document.Triangles, 3.5, -2, -2, 0, 3.5, 2, -2, 0, 3.5, 2, 10, 0)
	denoiseTestPut(document.Triangles[48:], 3.5, -2, -2, 0, 3.5, 2, 10, 0, 3.5, -2, 10, 0)
}

type denoiseCancelContext struct {
	done          chan struct{}
	checks, limit int
}

func (*denoiseCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *denoiseCancelContext) Done() <-chan struct{}   { return ctx.done }
func (*denoiseCancelContext) Value(any) any               { return nil }
func (ctx *denoiseCancelContext) Err() error {
	ctx.checks++
	if ctx.checks >= ctx.limit {
		if ctx.checks == ctx.limit {
			close(ctx.done)
		}

		return context.Canceled
	}

	return nil
}
