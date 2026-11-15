package spritepaint

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestViewportAttachmentTransferContract(t *testing.T) {
	t.Parallel()

	for _, linear := range []bool{false, true} {
		values := Viewport(100, 80, linear)
		if len(values) != 16 || math.Float32frombits(binary.LittleEndian.Uint32(values)) != 100 ||
			math.Float32frombits(binary.LittleEndian.Uint32(values[4:])) != 80 {
			t.Fatal("viewport ABI dimensions")
		}

		flag := float32(0)
		if linear {
			flag = 1
		}

		if math.Float32frombits(binary.LittleEndian.Uint32(values[8:])) != flag {
			t.Fatal("attachment transfer flag")
		}
	}
}
