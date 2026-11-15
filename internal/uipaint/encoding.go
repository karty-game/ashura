package uipaint

import (
	"encoding/binary"
	"math"
)

func bits(v float32) uint32 { return math.Float32bits(v) }

// Record packs rectangle, clip, color, border, UV and radius/border/image parameters.
func Record(dst []byte, rect, clip, color, border, uv, parameters [4]float32) {
	for block, values := range [6][4]float32{rect, clip, color, border, uv, parameters} {
		for i, value := range values {
			binary.LittleEndian.PutUint32(dst[block*16+i*4:], bits(value))
		}
	}
}
