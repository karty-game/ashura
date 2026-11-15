//go:build !js

package msdfcodec

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/go-text/typesetting/font"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/vector"
)

func TestRequestBounds(t *testing.T) {
	t.Parallel()

	for _, request := range []glyphRequest{
		{width: 513, height: 1}, {width: 1}, {left: 1<<20 + 1}, {bottom: -(1 << 20) - 1},
	} {
		if validRequests(goregular.TTF, []glyphRequest{request}) {
			t.Fatal("accepted invalid glyph request", request)
		}
	}

	if validRequests(nil, nil) || validRequests(goregular.TTF, make([]glyphRequest, 65536)) {
		t.Fatal("accepted invalid font/count")
	}

	oversized := make([]glyphRequest, 65)
	for index := range oversized {
		oversized[index] = glyphRequest{width: 512, height: 512}
	}

	if validRequests(goregular.TTF, oversized) {
		t.Fatal("accepted more than64MiB of pixels")
	}

	if _, err := glyphCount([]byte{0, 0}); !errors.Is(err, errInvalid) {
		t.Fatal("accepted truncated font directory", err)
	}
}

//nolint:paralleltest // Serial VM tests bound simultaneous library memory.
func TestUnionWindingMatchesOriginalOutline(t *testing.T) {
	request := glyphRequest{id: 621, width: 27, height: 32, left: -2, bottom: -4}

	pixels, err := rasterize(t.Context(), goregular.TTF, []glyphRequest{request})
	if err != nil || len(pixels) != 1 {
		t.Fatal(err)
	}

	face, err := font.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}

	id, exists := face.NominalGlyph('∪')
	if !exists || uint32(id) != request.id {
		t.Fatal("union glyph identity changed")
	}

	outline, exists := face.GlyphDataOutline(id)
	if !exists {
		t.Fatal("union outline missing")
	}

	cpu := outlineCoverage(outline, face.Upem(), request)
	checked := 0

	for index, value := range cpu.Pix {
		if value != 0 && value != 255 {
			continue
		}

		actual := pixels[0][index*4 : index*4+3]

		median := max(min(actual[0], actual[1]), min(max(actual[0], actual[1]), actual[2]))
		if (median >= 128) != (value >= 128) {
			t.Fatal("MSDF winding disagrees with solid outline coverage", index)
		}

		checked++
	}

	if checked < 700 {
		t.Fatal("insufficient independent outline coverage", checked)
	}
}

func outlineCoverage(outline font.GlyphOutline, upem uint16, request glyphRequest) *image.Alpha {
	width, height := int(request.width), int(request.height)
	raster := vector.NewRasterizer(width, height)
	scale := float32(fontSize) / float32(upem)
	top := request.bottom + height

	for _, segment := range outline.Segments {
		points := segment.Args
		for index := range points {
			points[index].X = points[index].X*scale - float32(request.left)
			points[index].Y = float32(top) - points[index].Y*scale
		}

		switch segment.Op {
		case 0:
			raster.ClosePath()
			raster.MoveTo(points[0].X, points[0].Y)
		case 1:
			raster.LineTo(points[0].X, points[0].Y)
		case 2:
			raster.QuadTo(points[0].X, points[0].Y, points[1].X, points[1].Y)
		case 3:
			raster.CubeTo(points[0].X, points[0].Y, points[1].X, points[1].Y, points[2].X, points[2].Y)
		}
	}

	raster.ClosePath()

	pixels := image.NewAlpha(image.Rect(0, 0, width, height))
	raster.Draw(pixels, pixels.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})

	return pixels
}

//nolint:paralleltest // Active cancellation uses one precompiled VM and a fixed deadline.
func TestActiveCancellationRejectsPartialFontAndFreshCallRecovers(t *testing.T) {
	runtime := wazero.NewRuntimeWithConfig(
		t.Context(),
		wazero.NewRuntimeConfigCompiler().WithMemoryLimitPages(4096).WithCloseOnContextDone(true),
	)
	defer func() { _ = runtime.Close(context.WithoutCancel(t.Context())) }()

	module, err := instantiate(t.Context(), runtime)
	if err != nil {
		t.Fatal(err)
	}

	requests := make([]glyphRequest, 712)
	for index := range requests {
		requests[index] = glyphRequest{id: 621, width: 27, height: 32, left: -2, bottom: -4}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()

	started := time.Now()

	pixels, err := canceledBatch(ctx, module, requests)
	if !errors.Is(err, context.DeadlineExceeded) || pixels != nil || time.Since(started) > time.Second {
		t.Fatal("active VM failed cancellation bound", err, time.Since(started))
	}

	if pixels, err = rasterize(t.Context(), goregular.TTF, requests[:1]); err != nil || len(pixels) != 1 {
		t.Fatal("fresh VM did not recover after cancellation", err)
	}

	if pixels, err = rasterize(t.Context(), []byte("not a font"), requests[:1]); err == nil || pixels != nil {
		t.Fatal("accepted malformed font", err)
	}

	if _, err = PrepareGoRegular(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("accepted expired preparation", err)
	}
}

func canceledBatch(ctx context.Context, module api.Module, requests []glyphRequest) (pixels [][]byte, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			if cause, ok := failure.(error); ok {
				err = cause
			} else {
				err = errInvalid
			}

			pixels = nil
		}

		if ctx.Err() != nil {
			err = ctx.Err()
			pixels = nil
		}
	}()

	return renderRequests(ctx, module, goregular.TTF, requests), nil
}

func TestFloatOutputClampsInfiniteDistances(t *testing.T) {
	t.Parallel()

	data := make([]byte, 12)
	for index, value := range []float32{float32(math.Inf(-1)), .5, float32(math.Inf(1))} {
		binary.LittleEndian.PutUint32(data[index*4:], math.Float32bits(value))
	}

	if got := rgbaPixels(data, 1, 1); !bytes.Equal(got, []byte{0, 128, 255, 255}) {
		t.Fatal(got)
	}
}
