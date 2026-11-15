package modeldata

import (
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
)

// Encode writes deterministic little-endian GPU-ready model containers.
// Opaque-only documents retain exact v1 bytes; masked materials select v2.
//
//nolint:gosec // All counts/bytes are prebounded by encodedSize and geometry admission; -1 texture slots use signed wire bits.
func Encode(document Document) ([]byte, error) {
	size, err := encodedSize(document)
	if err != nil {
		return nil, err
	}

	data := make([]byte, headerSize, size)
	copy(data, magic)

	wireVersion := documentVersion(document)
	binary.LittleEndian.PutUint32(data[8:], wireVersion)
	binary.LittleEndian.PutUint32(data[16:], uint32(len(document.Parts)))
	binary.LittleEndian.PutUint32(data[20:], uint32(len(document.Textures)))

	data = appendText(data, document.Source)
	for _, image := range document.Textures {
		data = binary.LittleEndian.AppendUint32(data, uint32(image.Role))
		data = binary.LittleEndian.AppendUint32(data, uint32(len(image.Data)))
		data = appendAligned(data, image.Data)
	}

	for _, part := range document.Parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return nil, err
		}

		data = appendText(data, part.Name)
		data = binary.LittleEndian.AppendUint32(data, uint32(len(part.Vertices)/meshdata.VertexStride))

		data = binary.LittleEndian.AppendUint32(data, uint32(len(part.Indices)/meshdata.IndexStride))
		data = appendMaterial(data, part.Material, wireVersion)
		data = append(data, part.Vertices...)

		data = append(data, part.Indices...)
		if len(data) > MaxBytes {
			return nil, errBounds
		}
	}

	binary.LittleEndian.PutUint32(data[12:], uint32(len(data)))

	if _, err := Decode(data); err != nil {
		return nil, err
	}

	return data, nil
}

//nolint:gosec // Encoder preflight bounds every string before this wire-length conversion.
func appendText(data []byte, value string) []byte {
	data = binary.LittleEndian.AppendUint32(data, uint32(len(value)))

	return appendAligned(data, []byte(value))
}

func appendAligned(data, value []byte) []byte {
	data = append(data, value...)
	for len(data)%wordSize != 0 {
		data = append(data, 0)
	}

	return data
}

//nolint:gosec // Length arithmetic is nonnegative and capped at 128 MiB before int conversion.
func encodedSize(document Document) (int, error) {
	if len(document.Parts) == 0 || len(document.Parts) > MaxParts || len(document.Textures) > MaxTextures ||
		len(document.Source) > maxText {
		return 0, errBounds
	}

	size := uint64(headerSize + wordSize + (len(document.Source)+wordSize-1)/wordSize*wordSize)
	for _, image := range document.Textures {
		size += uint64(2*wordSize) + (uint64(len(image.Data))+wordSize-1)/wordSize*wordSize
	}

	materialSize := materialWords
	if documentVersion(document) == maskVersion {
		materialSize += maskWords
	}

	for _, part := range document.Parts {
		if len(part.Name) > maxText {
			return 0, errBounds
		}

		size += uint64(
			(len(part.Name)+wordSize-1)/wordSize*wordSize+(partHeaderWords+materialSize)*wordSize,
		) + uint64(
			len(part.Vertices),
		) + uint64(
			len(part.Indices),
		)
	}

	if size > MaxBytes {
		return 0, errBounds
	}

	return int(size), nil
}

//nolint:gosec // Signed texture slots retain exact wire bits and are checked by complete model admission.
func appendMaterial(data []byte, material Material, wireVersion uint32) []byte {
	for _, value := range material.Color {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
	}

	for _, value := range []float32{material.Roughness, material.NormalStrength, material.AOStrength} {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
	}

	for _, index := range []int32{material.Albedo, material.Normal, material.AO} {
		data = binary.LittleEndian.AppendUint32(data, uint32(index))
	}

	var doubleSided uint32
	if material.DoubleSided {
		doubleSided = 1
	}

	data = binary.LittleEndian.AppendUint32(data, doubleSided)

	if wireVersion == maskVersion {
		var alphaMask uint32
		if material.AlphaMask {
			alphaMask = 1
		}

		data = binary.LittleEndian.AppendUint32(data, alphaMask)
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(material.AlphaCutoff))
	}

	return data
}
