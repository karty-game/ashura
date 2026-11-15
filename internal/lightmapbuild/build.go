//go:build !js

// Package lightmapbuild prepares charted model geometry and ray inputs offline.
//
//nolint:mnd,varnamelen // Numeric offsets encode the GPU ABI; coordinate names follow barycentric/BVH equations.
package lightmapbuild

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/xatlas"
)

// Options are build-only atlas settings. The result is one complete atlas page.
var errGeometry = errors.New("invalid prepared lightmap geometry")

type Options struct {
	Resolution, Padding uint32
	RNM                 bool
	PrecomputeTransport bool
	// PreserveGeometricNormals retains build-only ray origins separately from
	// interpolated shading normals. It does not change the published sidecar.
	PreserveGeometricNormals bool
	TransportSamples         uint32
}

// Result holds the remapped model and matching immutable bake inputs.
type Result struct {
	Model, Lightmap                      []byte
	GeometricNormals                     []byte
	Width, Height, Charts, CoveredTexels int
}

// Prepare unwraps the complete placed model and prepares bounded GPU input bytes.
func Prepare(ctx context.Context, data []byte, options Options) (Result, error) {
	options, err := normalizeTransportOptions(options)
	if err != nil {
		return Result{}, err
	}

	model, err := modeldata.Decode(data)
	if err != nil {
		return Result{}, err
	}

	for _, part := range model.Parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return Result{}, err
		}
	}

	session, err := xatlas.OpenSession(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = session.Close() }()

	atlas, err := session.Generate(ctx, model.Parts, options.Resolution, options.Padding)
	if err != nil {
		return Result{}, err
	}

	doc := lightmapdata.Document{Width: atlas.Width, Height: atlas.Height, UVs: make([][]byte, len(model.Parts))}
	doc.UVs = remapModel(&model, atlas)

	remapped, err := modeldata.Encode(model)
	if err != nil {
		return Result{}, err
	}

	doc.ModelSHA256 = sha256.Sum256(remapped)

	var geometricNormals []byte
	if err := prepareLighting(ctx, &doc, model, atlas, options, &geometricNormals); err != nil {
		return Result{}, err
	}

	output, err := lightmapdata.Encode(doc)
	if err != nil {
		return Result{}, err
	}

	covered := 0

	for offset := 12; offset < len(doc.Receivers); offset += 32 {
		if readFloat(doc.Receivers[offset:]) != 0 {
			covered++
		}
	}

	return Result{
		Model:            remapped,
		Lightmap:         output,
		GeometricNormals: geometricNormals,
		Width:            int(atlas.Width),
		Height:           int(atlas.Height),
		Charts:           int(atlas.Charts),
		CoveredTexels:    covered,
	}, nil
}

type vec [3]float32

func vertex(data []byte, index uint32, offset int) vec {
	start := int(index)*32 + offset

	return vec{readFloat(data[start:]), readFloat(data[start+4:]), readFloat(data[start+8:])}
}
func readFloat(data []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(data)) }
func putFloat(data []byte, value float32) {
	binary.LittleEndian.PutUint32(data, math.Float32bits(value))
}
func sub(a, b vec) vec { return vec{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func cross(a, b vec) vec {
	return vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func lengthSquared(v vec) float32 { return v[0]*v[0] + v[1]*v[1] + v[2]*v[2] }
func normalize(v vec) vec {
	d := float32(math.Sqrt(float64(lengthSquared(v))))
	if d == 0 {
		return vec{0, 1, 0}
	}

	return vec{v[0] / d, v[1] / d, v[2] / d}
}

func remapModel(model *modeldata.Document, atlas xatlas.Atlas) [][]byte {
	uvsByPart := make([][]byte, len(model.Parts))
	for i := range model.Parts {
		original := model.Parts[i].Vertices
		m := atlas.Meshes[i]
		vertices := make([]byte, len(m.Vertices)*meshdata.VertexStride)
		uvs := make([]byte, len(m.Vertices)*8)

		indices := make([]byte, len(m.Indices)*4)
		for j, v := range m.Vertices {
			copy(vertices[j*32:], original[int(v.Original)*32:int(v.Original+1)*32])
			putFloat(uvs[j*8:], v.UV[0]/float32(atlas.Width))
			putFloat(uvs[j*8+4:], v.UV[1]/float32(atlas.Height))
		}

		for j, v := range m.Indices {
			binary.LittleEndian.PutUint32(indices[j*4:], v)
		}

		model.Parts[i].Vertices = vertices
		model.Parts[i].Indices = indices
		uvsByPart[i] = uvs
	}

	return uvsByPart
}

func normalizeTransportOptions(options Options) (Options, error) {
	if (options.PrecomputeTransport || options.PreserveGeometricNormals) && !options.RNM {
		return options, fmt.Errorf("diffuse transport requires RNM: %w", errGeometry)
	}

	if options.TransportSamples == 0 {
		options.TransportSamples = 64
	}

	if options.TransportSamples < 16 || options.TransportSamples > 128 {
		return options, fmt.Errorf("invalid diffuse transport sample count: %w", errGeometry)
	}

	return options, nil
}

func prepareLighting(
	ctx context.Context,
	doc *lightmapdata.Document,
	model modeldata.Document,
	atlas xatlas.Atlas,
	options Options,
	geometricOutput *[]byte,
) error {
	var err error

	triangles := collectTriangles(model.Parts)
	if len(triangles) == 0 {
		return fmt.Errorf("model has no nondegenerate triangles: %w", errGeometry)
	}

	doc.Nodes, doc.Triangles = buildBVH(triangles)

	var geometricNormals []vec
	if options.PrecomputeTransport || options.PreserveGeometricNormals {
		geometricNormals = make([]vec, int(atlas.Width)*int(atlas.Height))
	}

	doc.Receivers, doc.Padding, doc.Frames, err = buildReceivers(ctx, model.Parts, atlas, options.RNM, geometricNormals)
	if err != nil {
		return err
	}

	if options.RNM {
		doc.Surfaces, err = buildSurfaces(triangles, model)
		if err != nil {
			return err
		}
	}

	if options.PrecomputeTransport {
		if err := buildTransport(ctx, doc, triangles, geometricNormals, options.TransportSamples); err != nil {
			return err
		}
	}

	if options.PreserveGeometricNormals {
		*geometricOutput = encodeGeometricNormals(geometricNormals)
	}

	return nil
}

// The auxiliary buffer is vec4f-aligned for WebGPU. Uncovered pixels stay zero.
func encodeGeometricNormals(normals []vec) []byte {
	data := make([]byte, len(normals)*16)
	for index, normal := range normals {
		for axis, value := range normal {
			putFloat(data[index*16+axis*4:], value)
		}
	}

	return data
}
