package ktx2data_test

import (
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

//go:embed testdata/rgba8.ktx2
var prepared []byte

func TestDecodePreparedPayloadWithoutRuntimeDependencies(t *testing.T) {
	t.Parallel()

	data, err := ktx2data.Decode(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if data.Width != 1 || data.Height != 1 || data.ColorSpace != texturedata.SRGB || len(data.Levels) != 1 || len(data.Levels[0]) != 4 {
		t.Fatalf("unexpected metadata: %+v", data)
	}
}

func TestKTX2RejectsMalformedPayloads(t *testing.T) {
	t.Parallel()

	for length := range prepared {
		if _, err := ktx2data.Decode(prepared[:length]); err == nil {
			t.Fatalf("accepted truncated length %d", length)
		}
	}

	const (
		formatOffset      = 12
		widthOffset       = 20
		levelsOffset      = 40
		compressionOffset = 44
		dfdOffset         = 48
		levelOffset       = 80
	)

	cases := []struct {
		name   string
		mutate func([]byte)
	}{
		{"unsupported format", func(data []byte) { binary.LittleEndian.PutUint32(data[formatOffset:], 0) }},
		{"oversized dimensions", func(data []byte) { binary.LittleEndian.PutUint32(data[widthOffset:], ^uint32(0)) }},
		{"too many mips", func(data []byte) { binary.LittleEndian.PutUint32(data[levelsOffset:], 2) }},
		{"supercompression", func(data []byte) { binary.LittleEndian.PutUint32(data[compressionOffset:], 1) }},
		{"bad descriptor", func(data []byte) { binary.LittleEndian.PutUint32(data[dfdOffset:], ^uint32(0)) }},
		{"overflowing level range", func(data []byte) { binary.LittleEndian.PutUint64(data[levelOffset:], ^uint64(0)) }},
		{"level overlaps header", func(data []byte) { binary.LittleEndian.PutUint64(data[levelOffset:], 0) }},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			data := append([]byte(nil), prepared...)
			fixture.mutate(data)

			if _, err := ktx2data.Decode(data); err == nil {
				t.Fatal("accepted malformed container")
			}
		})
	}
}
