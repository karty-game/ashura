//go:build !js

package impostor_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/impostor"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func TestBC1PreparedPairLoadsBeforeRunAndRejectsEncodingMismatch(t *testing.T) {
	t.Parallel()

	document := bc1FixtureDocument(t)

	good, err := impostordata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	// CPU admission remains legal before a backend has been opened. RGBA8 here
	// means no device was queried, not a known lack of BC compression support.
	if app.CompressedAlbedoFormat() != texture.RGBA8 {
		t.Fatal("fixture unexpectedly opened a backend")
	}

	asset, err := impostor.Prepared("bc1", good).Load(app)
	if err != nil {
		t.Fatal("pre-Run BC1 admission failed", err)
	}

	color, normal, _, _ := asset.RuntimeTextures()
	if texturedata.Encoding(color) != texture.BC1 || texturedata.Encoding(normal) != texture.BC1 ||
		texturedata.Space(color) != texture.SRGB || texturedata.Space(normal) != texture.Linear ||
		texturedata.ByteSize(color) != 32 || texturedata.ByteSize(normal) != 32 {
		t.Fatal("prepared block pair changed format, transfer or payload size")
	}

	if err := asset.Unload(); err != nil {
		t.Fatal(err)
	}

	raw, err := impostordata.Decode(fixtureAsset(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		change func(*impostordata.Document)
	}{
		{"raw-storage-with-blocks", func(value *impostordata.Document) { value.Storage = impostordata.StorageRGBA8 }},
		{"raw-color", func(value *impostordata.Document) { value.Color = raw.Color }},
		{"raw-normal", func(value *impostordata.Document) { value.NormalDepth = raw.NormalDepth }},
		{"rgb-color", func(value *impostordata.Document) { value.Color = alternateBC1Profile(value.Color, 132, false) }},
		{"rgb-normal", func(value *impostordata.Document) {
			value.NormalDepth = alternateBC1Profile(value.NormalDepth, 131, false)
		}},
		{"linear-color", func(value *impostordata.Document) { value.Color = alternateBC1Profile(value.Color, 133, true) }},
		{"srgb-normal", func(value *impostordata.Document) {
			value.NormalDepth = alternateBC1Profile(value.NormalDepth, 134, true)
		}},
	} {
		changed := document
		test.change(&changed)

		// Both containers are valid GPU-ready textures. The impostor's complete
		// role/storage contract must reject them before creating App resources.
		if _, err := ktx2.Decode(changed.Color); err != nil {
			t.Fatal("invalid alternate color fixture", test.name, err)
		}

		if _, err := ktx2.Decode(changed.NormalDepth); err != nil {
			t.Fatal("invalid alternate numeric fixture", test.name, err)
		}

		bad, err := impostordata.Encode(changed)
		if err != nil {
			t.Fatal(err)
		}

		if result, err := impostor.Prepared(test.name, bad).Load(app); err == nil || result != nil {
			t.Fatal("admitted descriptor/texture mismatch", test.name, err)
		}
	}

	asset, err = impostor.Prepared("bc1-after-rejection", good).Load(app)
	if err != nil {
		t.Fatal("rejected texture roles poisoned App admission", err)
	}

	if err := asset.Unload(); err != nil {
		t.Fatal(err)
	}
}

func bc1FixtureDocument(t *testing.T) impostordata.Document {
	t.Helper()

	document, err := impostordata.Decode(fixtureAsset(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, encoded := range []*[]byte{&document.Color, &document.NormalDepth} {
		*encoded, err = assetbuild.CompressBC1(t.Context(), *encoded, assetbuild.BlockCompressionOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	document.Storage = impostordata.StorageBC1
	document.Uniform = nil

	return document
}

func alternateBC1Profile(data []byte, format uint32, alpha bool) []byte {
	result := bytes.Clone(data)
	binary.LittleEndian.PutUint32(result[12:], format)
	descriptor := binary.LittleEndian.Uint32(result[48:])
	result[descriptor+14] = 1

	if format == 132 || format == 134 {
		result[descriptor+14] = 2
	}

	result[descriptor+31] = 0
	if alpha {
		result[descriptor+31] = 1
	}

	return result
}
