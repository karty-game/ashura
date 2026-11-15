// Package lightmapdata admits immutable, build-prepared lighting bake inputs.
// Geometry preparation, charting and receiver rasterization are build-only work.
package lightmapdata

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const (
	NodeStride          = 32
	TriangleStride      = 48
	ReceiverStride      = 32
	UVStride            = 8
	FrameStride         = 16
	SurfaceStride       = 16
	GIReceiverStride    = 32
	GIPatchStride       = 48
	GITransferStride    = 16
	MaxTransportBytes   = 128 << 20
	MaxGITransfers      = 8
	MaxDimension        = 2048
	MaxParts            = 4096
	MaxTriangles        = 1 << 20
	MaxNodes            = 1 << 20
	MaxVertices         = 1 << 20
	MaxBytes            = 256 << 20
	headerSize          = 104
	rnmHeaderSize       = 112
	transportHeaderSize = 144
	magic               = "KARTLM1\x00"
	scalarVersion       = 1
	rnmVersion          = 2
	transportVersion    = 3
)

var ErrFormat = errors.New("invalid prepared lightmap inputs")

// Document stores exact little-endian GPU bytes. Nodes contain min.xyz, first,
// max.xyz, count; count zero denotes adjacent children first and first+1.
// Triangles are three vec4 positions. Receivers are position.xyz, coverage,
// normal.xyz, chartID+1. Coverage is 0, 1, or 2 for double-sided receivers.
// Padding maps every output texel to a covered receiver index or UINT_MAX.
type Document struct {
	Width, Height                        uint32
	ModelSHA256                          [32]byte
	UVs                                  [][]byte
	Nodes, Triangles, Receivers, Padding []byte
	// Frames hold tangent.xyz and UV0 handedness for each covered receiver.
	// Surfaces hold mean linear material reflectance.rgb and double-sided flag
	// for each BVH-ordered triangle. Both arrays are present only for RNM inputs.
	Frames, Surfaces []byte
	// Prepared diffuse transport: pixel→compact receiver, per-receiver escaped
	// environment weights, physical reflector patches, and at most eight patch links.
	ReceiverGIIndex, GIReceivers, GIPatches, GITransfers []byte
	GISamples                                            uint32
	// GPUData owns the contiguous prepared arrays in the order above, including
	// Frames and Surfaces. Decode establishes this backing without repacking.
	GPUData []byte
}

// Decode verifies the complete envelope and all references before copying bytes.
func Decode(data []byte) (*Document, error) {
	if len(data) < headerSize || len(data) > MaxBytes || string(data[:8]) != magic {
		return nil, ErrFormat
	}

	version, header, err := documentHeader(data)
	if err != nil {
		return nil, err
	}

	var expected [32]byte
	copy(expected[:], data[72:104])

	if artifactChecksum(data) != expected {
		return nil, ErrFormat
	}

	document := &Document{Width: binary.LittleEndian.Uint32(data[12:]), Height: binary.LittleEndian.Uint32(data[16:])}
	copy(document.ModelSHA256[:], data[40:72])

	if version == transportVersion {
		document.GISamples = binary.LittleEndian.Uint32(data[128:])
	}

	parts := binary.LittleEndian.Uint32(data[20:])
	if parts == 0 || parts > MaxParts {
		return nil, ErrFormat
	}

	offset := header
	document.UVs = make([][]byte, parts)

	for index := range document.UVs {
		if offset > len(data)-4 {
			return nil, ErrFormat
		}

		size := binary.LittleEndian.Uint32(data[offset:])

		offset += 4
		if size > MaxBytes || int(size) > len(data)-offset {
			return nil, ErrFormat
		}

		document.UVs[index] = data[offset : offset+int(size)]
		offset += int(size)
	}

	gpuStart := offset

	fields, offset, err := readGPUFields(document, data, version, offset)
	if err != nil {
		return nil, err
	}

	if offset != len(data) || !validVersionFields(document, version) ||
		Validate(document) != nil {
		return nil, ErrFormat
	}

	ownPreparedArrays(document, data, gpuStart, fields)

	return document, nil
}

// Encode validates then writes a versioned, checksummed prepared artifact.
//
//nolint:gosec // Complete validation bounds every length below MaxBytes before uint32 encoding.
func Encode(document Document) ([]byte, error) {
	if err := Validate(&document); err != nil {
		return nil, err
	}

	data := make([]byte, encodedSize(&document))
	copy(data, magic)

	version, header := uint32(scalarVersion), headerSize
	if document.RNM() {
		version, header = rnmVersion, rnmHeaderSize

		binary.LittleEndian.PutUint32(data[104:], uint32(len(document.Frames)))
		binary.LittleEndian.PutUint32(data[108:], uint32(len(document.Surfaces)))
	}

	if document.Transport() {
		version, header = transportVersion, transportHeaderSize

		for i, field := range [][]byte{document.ReceiverGIIndex, document.GIReceivers, document.GIPatches, document.GITransfers} {
			binary.LittleEndian.PutUint32(data[112+i*4:], uint32(len(field)))
		}

		binary.LittleEndian.PutUint32(data[128:], document.GISamples)
	}

	for i, v := range []uint32{version, document.Width, document.Height, uint32(len(document.UVs)), uint32(len(document.Nodes)), uint32(len(document.Triangles)), uint32(len(document.Receivers)), uint32(len(document.Padding))} {
		binary.LittleEndian.PutUint32(data[8+i*4:], v)
	}

	copy(data[40:72], document.ModelSHA256[:])

	offset := header
	for _, coordinates := range document.UVs {
		binary.LittleEndian.PutUint32(data[offset:], uint32(len(coordinates)))
		offset += 4
		offset += copy(data[offset:], coordinates)
	}

	for _, field := range document.fields() {
		offset += copy(data[offset:], *field)
	}

	sum := artifactChecksum(data)
	copy(data[72:104], sum[:])

	return data, nil
}

func artifactChecksum(data []byte) [32]byte {
	hash := sha256.New()
	_, _ = hash.Write(data[:72])
	_, _ = hash.Write(data[headerSize:])

	var sum [32]byte
	copy(sum[:], hash.Sum(nil))

	return sum
}

// RNM reports whether this document contains the directional bake extension.
func (document *Document) RNM() bool { return document != nil && len(document.Frames) != 0 }

// Transport reports whether geometry-dependent diffuse transport was prepared offline.
func (document *Document) Transport() bool {
	return document != nil && len(document.ReceiverGIIndex) != 0
}

func (document *Document) fields() []*[]byte {
	return []*[]byte{
		&document.Nodes,
		&document.Triangles,
		&document.Receivers,
		&document.Padding,
		&document.Frames,
		&document.Surfaces,
		&document.ReceiverGIIndex,
		&document.GIReceivers,
		&document.GIPatches,
		&document.GITransfers,
	}
}

func documentHeader(data []byte) (uint32, int, error) {
	version := binary.LittleEndian.Uint32(data[8:])

	header := headerSize

	switch version {
	case transportVersion:
		header = transportHeaderSize
	case rnmVersion:
		header = rnmHeaderSize
	case scalarVersion:
	default:
		return 0, 0, ErrFormat
	}

	if len(data) < header {
		return 0, 0, ErrFormat
	}

	if version == transportVersion {
		for _, value := range data[132:144] {
			if value != 0 {
				return 0, 0, ErrFormat
			}
		}
	}

	return version, header, nil
}

func readGPUFields(document *Document, data []byte, version uint32, offset int) ([]*[]byte, int, error) {
	fields := document.fields()

	lengths := []int{24, 28, 32, 36}
	if version >= rnmVersion {
		lengths = append(lengths, 104, 108)
		if version == transportVersion {
			lengths = append(lengths, 112, 116, 120, 124)
		} else {
			fields = fields[:6]
		}
	} else {
		fields = fields[:4]
	}

	for index, field := range fields {
		size := binary.LittleEndian.Uint32(data[lengths[index]:])
		if size > MaxBytes || int(size) > len(data)-offset {
			return nil, 0, ErrFormat
		}

		*field = data[offset : offset+int(size)]
		offset += int(size)
	}

	return fields, offset, nil
}

func validVersionFields(document *Document, version uint32) bool {
	if version >= rnmVersion && (len(document.Frames) == 0 || len(document.Surfaces) == 0) {
		return false
	}

	return version != transportVersion || document.Transport()
}

func ownPreparedArrays(document *Document, data []byte, gpuStart int, fields []*[]byte) {
	for index := range document.UVs {
		document.UVs[index] = append([]byte(nil), document.UVs[index]...)
	}

	document.GPUData = append([]byte(nil), data[gpuStart:]...)
	offset := 0

	for _, field := range fields {
		size := len(*field)
		*field = document.GPUData[offset : offset+size : offset+size]
		offset += size
	}
}
