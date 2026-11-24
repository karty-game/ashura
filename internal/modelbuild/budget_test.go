//go:build !js

package modelbuild

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"

	"github.com/karty-game/ashura/internal/modeldata"
)

func TestTextureExpansionBudgetPrecedesPixelDecode(t *testing.T) {
	t.Parallel()
	// A valid PNG IHDR admits a 4096x4096 image via DecodeConfig, but there are
	// deliberately no pixel data. The cumulative budget must fail before decoding.
	png := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	ihdr := []byte{'I', 'H', 'D', 'R', 0, 0, 16, 0, 0, 0, 16, 0, 8, 6, 0, 0, 0}
	png = binary.BigEndian.AppendUint32(png, 13)
	png = append(png, ihdr...)
	png = binary.BigEndian.AppendUint32(png, crc32.ChecksumIEEE(ihdr))
	png = binary.BigEndian.AppendUint32(png, 0)
	png = append(png, []byte("IDAT")...)
	png = binary.BigEndian.AppendUint32(png, crc32.ChecksumIEEE([]byte("IDAT")))

	build := compiler{binary: png, textureBytes: 80 << 20, document: gltf{
		Images: []gltfImage{{BufferView: new(int), MimeType: "image/png"}}, BufferViews: []bufferView{{ByteLength: len(png)}},
	}}
	if _, err := build.image(0, modeldata.Albedo); !errors.Is(err, errBounds) {
		t.Fatalf("wanted budget rejection before missing-pixel decode, got %v", err)
	}
}
