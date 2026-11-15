// Package lightmapfixture provides a small visibility and chart-padding fixture.
package lightmapfixture

import (
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
)

// Document has one blocked receiver, one lit receiver, its gutter, and an empty
// texel. A horizontal triangle at y=1 occludes only the receiver at x=-1.
//
//nolint:mnd // Four pixels explicitly exercise blocked, lit, gutter and empty output.
func Document() lightmapdata.Document {
	data := lightmapdata.Document{
		Width:       4,
		Height:      1,
		ModelSHA256: [32]byte{1},
		UVs:         [][]byte{make([]byte, 3*lightmapdata.UVStride)},
		Nodes:       make([]byte, lightmapdata.NodeStride),
		Triangles:   make([]byte, lightmapdata.TriangleStride),
		Receivers:   make([]byte, 4*lightmapdata.ReceiverStride),
		Padding:     make([]byte, 4*4),
	}
	put(data.Nodes, 0, -2, 1, -2)
	put(data.Nodes, 16, 0, 1, 2)
	binary.LittleEndian.PutUint32(data.Nodes[28:], 1)
	put(data.Triangles, 0, -2, 1, -2)
	put(data.Triangles, 16, 0, 1, -2)
	put(data.Triangles, 32, 0, 1, 2)
	put(data.Receivers, 0, -1, 0, 0, 1, 0, 1, 0, 1)
	put(data.Receivers, 32, 1, 0, 0, 1, 0, 1, 0, 1)

	for i, value := range []uint32{0, 1, 1, math.MaxUint32} {
		binary.LittleEndian.PutUint32(data.Padding[i*4:], value)
	}

	return data
}

func put(data []byte, offset int, values ...float32) {
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[offset+i*4:], math.Float32bits(v))
	}
}

// RNMDocument extends Document with a UV0 frame at its two covered receivers and
// neutral diffuse reflectance for the occluder. The v1 fixture remains unchanged.
func RNMDocument() lightmapdata.Document {
	data := Document()
	data.Frames = make([]byte, 4*lightmapdata.FrameStride)
	data.Surfaces = make([]byte, lightmapdata.SurfaceStride)
	put(data.Frames, 0, 1, 0, 0, 1)
	put(data.Frames, 16, 1, 0, 0, 1)
	put(data.Surfaces, 0, .5, .5, .5, 0)

	return data
}

// TransportDocument adds one precomputed red reflector and open-sky coefficients
// to RNMDocument, preserving the original geometric visibility fixture.
func TransportDocument() lightmapdata.Document {
	data := RNMDocument()
	data.GISamples = 64

	data.ReceiverGIIndex = make([]byte, 16)
	for i, value := range []uint32{0, 1, math.MaxUint32, math.MaxUint32} {
		binary.LittleEndian.PutUint32(data.ReceiverGIIndex[i*4:], value)
	}

	data.GIReceivers = make([]byte, 64)
	binary.LittleEndian.PutUint32(data.GIReceivers[4:], 1)
	binary.LittleEndian.PutUint32(data.GIReceivers[32:], 1)
	put(data.GIReceivers, 40, .3, .3, .3)
	data.GIPatches = make([]byte, 48)
	put(data.GIPatches, 0, -1, 1, 0, 0, 0, -1, 0, 0, .95, 0, 0, 0)
	data.GITransfers = make([]byte, 16)
	put(data.GITransfers, 4, .2, .2, .2)

	return data
}
