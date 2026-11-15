//go:build !js

//nolint:varnamelen // Geometry fixture coordinates use conventional short names.
package xatlas

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestPhysicalDensityPagesAndSession(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, err := OpenSession(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = session.Close() }()

	parts := []modeldata.Part{pageTestQuad("first", 0, 1), pageTestQuad("second", 3, 1)}
	options := PageOptions{TexelsPerUnit: 16, Resolution: 32, Padding: 4, MaxPages: 8}

	pages, err := session.GeneratePages(ctx, parts, options)
	if err != nil {
		t.Fatal(err)
	}

	if len(pages) < 2 {
		t.Fatalf("expected multiple fixed-density pages, got %d", len(pages))
	}

	triangles := 0

	for _, page := range pages {
		if page.Width != 32 || page.Height != 32 || page.TexelsPerUnit != 16 || page.Charts == 0 {
			t.Fatal("bad page metadata")
		}

		for i, mesh := range page.Meshes {
			triangles += len(mesh.Indices) / 3
			for _, v := range mesh.Vertices {
				if int(v.Original) >= len(parts[page.PartIndices[i]].Vertices)/32 {
					t.Fatal("source remap escaped")
				}
			}
		}
	}

	if triangles != 4 {
		t.Fatalf("triangle membership changed: %d", triangles)
	}

	again, err := session.GeneratePages(ctx, parts, options)
	if err != nil {
		t.Fatal(err)
	}

	if !pagesEqual(pages, again) {
		t.Fatal("nondeterministic physical packing")
	}

	tiny, err := session.GeneratePages(ctx, []modeldata.Part{pageTestQuad("sub-epsilon valid surface", -5.6, .0001)}, options)
	if err != nil {
		t.Fatal(err)
	}

	tinyTriangles := 0

	for _, page := range tiny {
		for _, mesh := range page.Meshes {
			tinyTriangles += len(mesh.Indices) / 3
		}
	}

	if tinyTriangles != 2 {
		t.Fatal("valid clipped slivers disappeared from chart output")
	}

	options.MaxPages = 1
	if _, err = session.GeneratePages(ctx, parts, options); err == nil {
		t.Fatal("accepted page overflow")
	}

	options.MaxPages = 8
	if _, err = session.GeneratePages(ctx, []modeldata.Part{pageTestQuad("oversized", 0, 100)}, options); err == nil {
		t.Fatal("silently reduced oversized-chart density")
	}

	canceled, stop := context.WithCancel(ctx)
	stop()

	if _, err = session.GeneratePages(canceled, parts, options); err == nil {
		t.Fatal("accepted canceled generation")
	}

	if err = session.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err = session.GeneratePages(ctx, parts, options); err == nil {
		t.Fatal("used closed helper")
	}
}

func pageTestQuad(name string, x, size float32) modeldata.Part {
	part := modeldata.Part{
		Name:     name,
		Material: modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Roughness: .5, Albedo: -1, Normal: -1, AO: -1},
	}

	for i, p := range [][3]float32{{x, 0, 0}, {x + size, 0, 0}, {x + size, 0, size}, {x, 0, size}} {
		uv := [4][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
		for _, v := range []float32{p[0], p[1], p[2], 0, 1, 0, uv[i][0], uv[i][1]} {
			part.Vertices = binary.LittleEndian.AppendUint32(part.Vertices, math.Float32bits(v))
		}
	}

	for _, i := range []uint32{0, 2, 1, 0, 3, 2} {
		part.Indices = binary.LittleEndian.AppendUint32(part.Indices, i)
	}

	return part
}

func pagesEqual(a, b []Page) bool {
	if len(a) != len(b) {
		return false
	}

	for i, x := range a {
		y := b[i]
		if x.Width != y.Width || x.Height != y.Height || x.Charts != y.Charts || len(x.Meshes) != len(y.Meshes) ||
			len(x.PartIndices) != len(y.PartIndices) {
			return false
		}

		for j := range x.PartIndices {
			if x.PartIndices[j] != y.PartIndices[j] {
				return false
			}
		}

		if !bytes.Equal(words(x.Image), words(y.Image)) {
			return false
		}

		for j, m := range x.Meshes {
			n := y.Meshes[j]
			if len(m.Vertices) != len(n.Vertices) || !bytes.Equal(words(m.Indices), words(n.Indices)) {
				return false
			}

			for k, v := range m.Vertices {
				if v != n.Vertices[k] {
					return false
				}
			}
		}
	}

	return true
}
func words(v []uint32) []byte {
	out := make([]byte, 0, len(v)*4)
	for _, x := range v {
		out = binary.LittleEndian.AppendUint32(out, x)
	}

	return out
}

func TestMultipageDecoderRejectsMalformedAndMissingGeometry(t *testing.T) {
	t.Parallel()

	part := pageTestQuad("quad", 0, 1)
	options := PageOptions{TexelsPerUnit: 16, Resolution: 32, Padding: 4, MaxPages: 8}

	encoded := words([]uint32{0x32584f4b, 1, 32, 32, 1, 1, math.Float32bits(16), 4, 6})
	for i, uv := range [][2]float32{{4, 4}, {20, 4}, {20, 20}, {4, 20}} {
		encoded = append(encoded, words([]uint32{uint32(i), 0, 0, math.Float32bits(uv[0]), math.Float32bits(uv[1])})...)
	}

	encoded = append(encoded, part.Indices...)
	image := make([]uint32, 32*32)
	image[4*32+4] = 0x80000000

	encoded = append(encoded, words(image)...)
	if _, err := decodePages(encoded, []modeldata.Part{part}, options); err != nil {
		t.Fatal(err)
	}

	for n := range encoded {
		if _, err := decodePages(encoded[:n], []modeldata.Part{part}, options); err == nil {
			t.Fatalf("accepted truncated helper result%d", n)
		}
	}

	mutations := map[string]func([]byte){
		"pages":               func(b []byte) { binary.LittleEndian.PutUint32(b[20:], 65) },
		"density":             func(b []byte) { binary.LittleEndian.PutUint32(b[24:], math.Float32bits(float32(math.NaN()))) },
		"page-reference":      func(b []byte) { binary.LittleEndian.PutUint32(b[44:], 1) },
		"uv":                  func(b []byte) { binary.LittleEndian.PutUint32(b[48:], math.Float32bits(33)) },
		"source-reference":    func(b []byte) { binary.LittleEndian.PutUint32(b[36:], 4) },
		"triangle-membership": func(b []byte) { binary.LittleEndian.PutUint32(b[116:], 2) },
		"image-chart":         func(b []byte) { binary.LittleEndian.PutUint32(b[140:], 0x80000002) },
		"density-loss": func(b []byte) {
			for i := range 4 {
				for axis := range 2 {
					offset := 48 + i*20 + axis*4
					v := math.Float32frombits(binary.LittleEndian.Uint32(b[offset:]))
					binary.LittleEndian.PutUint32(b[offset:], math.Float32bits(v*.5))
				}
			}
		},
		"omitted-surface": func(b []byte) {
			for i := range 4 {
				binary.LittleEndian.PutUint32(b[40+i*20:], math.MaxUint32)
				binary.LittleEndian.PutUint32(b[44+i*20:], math.MaxUint32)
			}

			clear(b[140:])
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := bytes.Clone(encoded)
			mutate(b)

			if _, err := decodePages(b, []modeldata.Part{part}, options); err == nil {
				t.Fatal("accepted malformed helper result")
			}
		})
	}
}
