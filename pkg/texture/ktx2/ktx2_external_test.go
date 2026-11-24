package ktx2_test

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
)

//go:embed testdata/rgba8.ktx2
var prepared []byte

func TestPreparedKTX2Asset(t *testing.T) {
	t.Parallel()

	data, err := ktx2.Decode(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if data.Width != 1 || data.Height != 1 || data.ColorSpace != texture.SRGB || len(data.Levels) != 1 {
		t.Fatalf("unexpected texture metadata: %+v", data)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	image, err := ktx2.Albedo("test.ktx2", prepared).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	if width, height := image.Size(); width != 1 || height != 1 {
		t.Fatalf("unexpected dimensions: %dx%d", width, height)
	}

	if _, err := ktx2.Normal("wrong-role.ktx2", prepared).Load(app); !errors.Is(err, ktx2.ErrFormat) {
		t.Fatalf("role mismatch must match the public format error: %v", err)
	}

	if err := app.UnloadTexture(image); err != nil {
		t.Fatal(err)
	}
}

func TestKTX2RejectsMalformedPayloads(t *testing.T) {
	t.Parallel()

	for length := range prepared {
		if _, err := ktx2.Decode(prepared[:length]); !errors.Is(err, ktx2.ErrBounds) {
			t.Fatalf("truncated length %d must return ErrBounds: %v", length, err)
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

			if _, err := ktx2.Decode(data); err == nil {
				t.Fatal("accepted malformed container")
			}
		})
	}
}
