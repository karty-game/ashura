//go:build !js

// Package xatlas invokes the build-only pinned UV chart generator.
//

package xatlas

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

var errFormat = errors.New("invalid xatlas input/output")

// Vertex maps a generated lightmap seam vertex to the original mesh.
type Vertex struct {
	Original uint32
	Chart    int32
	UV       [2]float32
}

// Mesh contains xatlas's remapping for one original primitive.
type Mesh struct {
	Vertices []Vertex
	Indices  []uint32
}

// Atlas keeps one page and the exact chart occupancy/padding image.
type Atlas struct {
	Width, Height, Charts uint32
	Meshes                []Mesh
	Image                 []uint32
}

// Generate runs the embedded offline library and validates its bounded atlas result.
func Generate(ctx context.Context, parts []modeldata.Part, resolution, padding uint32) (Atlas, error) {
	if len(parts) == 0 || len(parts) > modeldata.MaxParts || resolution < 16 || resolution > 2048 || padding > 16 {
		return Atlas{}, fmt.Errorf("xatlas invalid input bounds: %w", errFormat)
	}

	session, err := OpenSession(ctx)
	if err != nil {
		return Atlas{}, err
	}
	defer func() { _ = session.Close() }()

	return session.Generate(ctx, parts, resolution, padding)
}

// Generate unwraps one bounded atlas through the embedded library. The internal
// byte boundary retains complete input/output admission.
//
//nolint:gosec // Mesh/model limits admit counts before wasm32 serialization.
func (session *Session) Generate(ctx context.Context, parts []modeldata.Part, resolution, padding uint32) (Atlas, error) {
	if ctx == nil || session == nil || session.wasm == nil || len(parts) == 0 || len(parts) > modeldata.MaxParts ||
		resolution < 16 || resolution > 2048 || padding > 16 {
		return Atlas{}, fmt.Errorf("xatlas invalid input bounds: %w", errFormat)
	}

	if err := ctx.Err(); err != nil {
		return Atlas{}, err
	}

	var input bytes.Buffer
	for _, word := range []uint32{0x3158414b, uint32(len(parts)), resolution, padding} {
		_ = binary.Write(&input, binary.LittleEndian, word)
	}

	var vertexCount, indexCount uint64

	for _, part := range parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return Atlas{}, err
		}

		vertexCount += uint64(len(part.Vertices) / meshdata.VertexStride)

		indexCount += uint64(len(part.Indices) / 4)
		if vertexCount > meshdata.MaxVertices || indexCount > meshdata.MaxIndices ||
			input.Len()+len(part.Vertices)+len(part.Indices)+8 > modeldata.MaxBytes {
			return Atlas{}, errFormat
		}

		_ = binary.Write(&input, binary.LittleEndian, uint32(len(part.Vertices)/meshdata.VertexStride))
		_ = binary.Write(&input, binary.LittleEndian, uint32(len(part.Indices)/4))
		input.Write(part.Vertices)
		input.Write(part.Indices)
	}

	output, err := session.wasm.execute(ctx, input.Bytes())
	if err != nil {
		return Atlas{}, err
	}

	return decode(output, parts)
}

type reader struct {
	data   []byte
	offset int
	err    error
}

func (input *reader) word() uint32 {
	if input.err != nil || len(input.data)-input.offset < 4 {
		input.err = io.ErrUnexpectedEOF

		return 0
	}

	v := binary.LittleEndian.Uint32(input.data[input.offset:])
	input.offset += 4

	return v
}

//nolint:gosec // Validated counts are bounded by prepared geometry and the finite helper output.
func decode(data []byte, parts []modeldata.Part) (Atlas, error) {
	input := reader{data: data}
	if input.word() != 0x31584f4b || input.word() != uint32(len(parts)) {
		return Atlas{}, fmt.Errorf("xatlas invalid output header: %w", errFormat)
	}

	atlas := Atlas{Width: input.word(), Height: input.word(), Charts: input.word()}
	if atlas.Width == 0 || atlas.Height == 0 || atlas.Width > 2048 || atlas.Height > 2048 || atlas.Charts == 0 ||
		atlas.Charts > meshdata.MaxIndices/3 {
		return Atlas{}, fmt.Errorf("xatlas output bounds: %w", errFormat)
	}

	atlas.Meshes = make([]Mesh, len(parts))
	totalVertices := uint64(0)

	for index := range atlas.Meshes {
		mesh, err := decodeMesh(&input, parts[index], atlas.Width, atlas.Height, atlas.Charts)
		if err != nil {
			return Atlas{}, err
		}

		totalVertices += uint64(len(mesh.Vertices))
		if totalVertices > meshdata.MaxVertices {
			return Atlas{}, errFormat
		}

		atlas.Meshes[index] = mesh
	}

	texels := uint64(atlas.Width) * uint64(atlas.Height)
	if texels*4 != uint64(len(data)-input.offset) {
		return Atlas{}, fmt.Errorf("xatlas image output bounds: %w", errFormat)
	}

	atlas.Image = make([]uint32, texels)
	for i := range atlas.Image {
		atlas.Image[i] = input.word()
		if atlas.Image[i] != 0 && atlas.Image[i]&0x1fffffff >= atlas.Charts {
			return Atlas{}, fmt.Errorf("xatlas invalid chart image: %w", errFormat)
		}
	}

	if input.err != nil {
		return Atlas{}, input.err
	}

	return atlas, nil
}
func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

//nolint:gosec // The output vertex/index counts are bounded before allocation and signed chart -1 is an explicit wire sentinel.
func decodeMesh(input *reader, part modeldata.Part, width, height, charts uint32) (Mesh, error) {
	count, indices := input.word(), input.word()
	if count == 0 || count > meshdata.MaxVertices || indices != uint32(len(part.Indices)/4) ||
		uint64(count)*16+uint64(indices)*4 > uint64(len(input.data)-input.offset) {
		return Mesh{}, errFormat
	}

	mesh := Mesh{Vertices: make([]Vertex, count), Indices: make([]uint32, indices)}
	for index := range mesh.Vertices {
		value := &mesh.Vertices[index]
		value.Original = input.word()
		value.Chart = int32(input.word())

		value.UV = [2]float32{math.Float32frombits(input.word()), math.Float32frombits(input.word())}
		if value.Original >= uint32(len(part.Vertices)/meshdata.VertexStride) || value.Chart < -1 || int64(value.Chart) >= int64(charts) ||
			!finite(value.UV[0]) ||
			!finite(value.UV[1]) ||
			value.UV[0] < 0 ||
			value.UV[1] < 0 ||
			value.UV[0] > float32(width) ||
			value.UV[1] > float32(height) {
			return Mesh{}, errFormat
		}
	}

	for index := range mesh.Indices {
		mesh.Indices[index] = input.word()
		if mesh.Indices[index] >= count {
			return Mesh{}, errFormat
		}
	}

	return mesh, input.err
}
