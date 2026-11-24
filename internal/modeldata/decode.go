package modeldata

import (
	"encoding/binary"
	"math"
	"unicode/utf8"

	"github.com/karty-game/ashura/internal/meshdata"
)

type reader struct {
	data    []byte
	offset  int
	version uint32
	err     error
}

// Decode admits container structure and material references. Geometry byte
// contents are admitted once by meshdata.FromOwned before creating resources.
// Returned slices reference the caller's immutable owned container.
func Decode(data []byte) (Document, error) {
	if !validHeader(data) {
		return Document{}, errFormat
	}

	parts, textures := binary.LittleEndian.Uint32(data[16:]), binary.LittleEndian.Uint32(data[20:])
	if parts == 0 || parts > MaxParts || textures > MaxTextures {
		return Document{}, errBounds
	}

	input := reader{data: data, offset: headerSize, version: binary.LittleEndian.Uint32(data[8:])}

	document := Document{Source: input.text(), Textures: make([]Texture, textures), Parts: make([]Part, parts)}
	for index := range document.Textures {
		document.Textures[index] = Texture{Role: TextureRole(input.word()), Data: input.bytes(int(input.word()))}
		if document.Textures[index].Role > Occlusion || len(document.Textures[index].Data) == 0 {
			return Document{}, errFormat
		}
	}

	var vertexCount, indexCount uint64

	for index := range document.Parts {
		part := &document.Parts[index]
		part.Name = input.text()
		vertices, indices := input.word(), input.word()
		vertexCount += uint64(vertices)

		indexCount += uint64(indices)
		if vertices == 0 || indices == 0 || vertexCount > meshdata.MaxVertices || indexCount > meshdata.MaxIndices {
			return Document{}, errBounds
		}

		part.Material = input.material()
		if input.version == boundsVersion {
			part.Bounds.Valid = true
			for axis := range part.Bounds.Minimum {
				part.Bounds.Minimum[axis] = math.Float32frombits(input.word())
			}

			for axis := range part.Bounds.Maximum {
				part.Bounds.Maximum[axis] = math.Float32frombits(input.word())
			}
		}

		part.Vertices = input.bytes(int(vertices) * meshdata.VertexStride)

		part.Indices = input.bytes(int(indices) * meshdata.IndexStride)
		if !validMaterial(part.Material, document.Textures) {
			return Document{}, errFormat
		}
	}

	if input.err != nil || input.offset != len(data) || document.Source == "" {
		return Document{}, errFormat
	}

	return document, nil
}

func (input *reader) word() uint32 {
	data := input.bytes(wordSize)
	if len(data) != wordSize {
		return 0
	}

	return binary.LittleEndian.Uint32(data)
}

func (input *reader) bytes(size int) []byte {
	if input.err != nil || size < 0 || size > len(input.data)-input.offset {
		input.err = errFormat

		return nil
	}

	data := input.data[input.offset : input.offset+size]
	input.offset += size

	padding := (wordSize - size%wordSize) % wordSize
	if padding > len(input.data)-input.offset {
		input.err = errFormat

		return nil
	}

	for _, value := range input.data[input.offset : input.offset+padding] {
		if value != 0 {
			input.err = errFormat
		}
	}

	input.offset += padding

	return data
}

func (input *reader) text() string {
	size := input.word()
	if size > maxText {
		input.err = errBounds

		return ""
	}

	data := input.bytes(int(size))
	if !utf8.Valid(data) {
		input.err = errFormat
	}

	return string(data)
}

//nolint:gosec // Wire uint32 words intentionally represent signed int32 texture slots, validated before resource creation.
func (input *reader) material() Material {
	var values [materialWords]uint32
	for index := range values {
		values[index] = input.word()
	}

	if values[10] > 1 {
		input.err = errFormat
	}

	material := Material{
		Color: [4]float32{
			math.Float32frombits(values[0]),
			math.Float32frombits(values[1]),
			math.Float32frombits(values[2]),
			math.Float32frombits(values[3]),
		},
		Roughness: math.Float32frombits(
			values[4],
		),
		NormalStrength: math.Float32frombits(values[5]),
		AOStrength:     math.Float32frombits(values[6]),
		Albedo:         int32(values[7]),
		Normal:         int32(values[8]),
		AO:             int32(values[9]),
		DoubleSided:    values[10] == 1,
	}

	if input.version >= maskVersion {
		mask := input.word()
		if mask > 1 {
			input.err = errFormat
		}

		material.AlphaMask = mask == 1
		material.AlphaCutoff = math.Float32frombits(input.word())
	}

	return material
}

func validHeader(data []byte) bool {
	if len(data) < headerSize || len(data) > MaxBytes || string(data[:len(magic)]) != magic ||
		uint64(binary.LittleEndian.Uint32(data[12:])) != uint64(len(data)) {
		return false
	}

	wireVersion := binary.LittleEndian.Uint32(data[8:])

	return wireVersion >= version && wireVersion <= boundsVersion
}
