//go:build !js

//nolint:gosec // Fixed font profile and pinned wasm32 ABI; requests are bounded before narrowing.
package msdfcodec

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"golang.org/x/image/font/gofont/goregular"
)

func encodePair(records []glyphRecord, pixels [][]byte) (Pair, error) {
	if len(records) != len(pixels) || len(records) > 65535 {
		return Pair{}, errInvalid
	}

	atlas := make([]byte, atlasSize*atlasSize*4)

	metrics := append([]byte(nil), "AFNT0001"...)
	for _, value := range []uint32{atlasSize, atlasSize, fontSize, 6, uint32(len(records))} {
		metrics = binary.LittleEndian.AppendUint32(metrics, value)
	}

	metrics = append(metrics, make([]byte, 64)...)

	for glyphID, record := range records {
		metrics = binary.LittleEndian.AppendUint32(metrics, uint32(glyphID))
		for _, value := range []float32{float32(record.left), float32(record.top), float32(record.width), float32(record.height),
			float32(record.atlasX) / atlasSize, float32(record.atlasY) / atlasSize, float32(record.width) / atlasSize, float32(record.height) / atlasSize} {
			metrics = binary.LittleEndian.AppendUint32(metrics, math.Float32bits(value))
		}

		if len(pixels[glyphID]) != record.width*record.height*4 {
			return Pair{}, errInvalid
		}

		for row := range record.height {
			copy(atlas[((record.atlasY+row)*atlasSize+record.atlasX)*4:],
				pixels[glyphID][row*record.width*4:(row+1)*record.width*4])
		}
	}

	fontHash, atlasHash := sha256.Sum256(goregular.TTF), sha256.Sum256(atlas)
	copy(metrics[28:60], fontHash[:])
	copy(metrics[60:92], atlasHash[:])

	return Pair{Atlas: atlas, Metrics: metrics}, nil
}
