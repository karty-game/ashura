package ktx2data_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestBC1AlphaProfileRequiresMatchingDescriptor(t *testing.T) {
	t.Parallel()

	for _, srgb := range []bool{false, true} {
		data := bytes.Clone(bc1SRGB)

		format, space, transfer := uint32(133), texturedata.Linear, byte(1)
		if srgb {
			format, space, transfer = 134, texturedata.SRGB, 2
		}

		binary.LittleEndian.PutUint32(data[12:], format)
		descriptor := binary.LittleEndian.Uint32(data[48:])
		data[descriptor+14], data[descriptor+31] = transfer, 1

		image, err := ktx2data.Decode(data)
		if err != nil || image.Format != texturedata.BC1 || image.ColorSpace != space || !ktx2data.BC1Alpha(data) {
			t.Fatal("BC1 alpha profile rejected", image, err)
		}

		original, err := ktx2data.Decode(bc1SRGB)
		if err != nil || !bytes.Equal(original.Levels[0], image.Levels[0]) {
			t.Fatal("alpha metadata changed native block bytes", err)
		}

		data[descriptor+31] = 0
		if _, err := ktx2data.Decode(data); err == nil {
			t.Fatal("RGBA Vulkan format accepted RGB-only descriptor")
		}

		data[descriptor+31] = 1
		binary.LittleEndian.PutUint32(data[12:], 132)

		data[descriptor+14] = 2
		if _, err := ktx2data.Decode(data); err == nil {
			t.Fatal("RGB Vulkan format accepted alpha-present descriptor")
		}
	}

	if ktx2data.BC1Alpha(bc1SRGB) || ktx2data.BC1Alpha(bc7) || ktx2data.BC1Alpha(nil) {
		t.Fatal("unrelated or missing profile identified as BC1 alpha")
	}

	for length := range 80 {
		if ktx2data.BC1Alpha(bc1SRGB[:length]) {
			t.Fatal("truncated header identified as BC1 alpha")
		}
	}
}
