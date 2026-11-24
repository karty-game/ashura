//go:build !js

package impostor_test

import (
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/impostor"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestMobilePreparedPairAdmitsOnlyMatchingNativeStorage(t *testing.T) {
	t.Parallel()

	document, err := impostordata.Decode(fixtureAsset(t))
	if err != nil {
		t.Fatal(err)
	}

	for _, encoded := range []*[]byte{&document.Color, &document.NormalDepth} {
		*encoded, err = assetbuild.CompressETC2RGBA(t.Context(), *encoded, assetbuild.BlockCompressionOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	document.Storage, document.Uniform = impostordata.StorageETC2RGBA, nil

	encoded, err := impostordata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	asset, err := impostor.Prepared("mobile", encoded).Load(app)
	if err != nil {
		t.Fatal("mobile storage admission before Run", err)
	}

	color, normal, _, _ := asset.RuntimeTextures()
	if texturedata.Encoding(color) != texture.ETC2RGBA || texturedata.Encoding(normal) != texture.ETC2RGBA ||
		texturedata.Space(color) != texture.SRGB || texturedata.Space(normal) != texture.Linear ||
		texturedata.ByteSize(color) != 64 || texturedata.ByteSize(normal) != 64 {
		t.Fatal("mobile atlas format, role or block bounds changed")
	}

	if err := asset.Unload(); err != nil {
		t.Fatal(err)
	}

	for _, storage := range []impostordata.Storage{impostordata.StorageRGBA8, impostordata.StorageBC1} {
		wrong := document
		wrong.Storage = storage

		encoded, err := impostordata.Encode(wrong)
		if err != nil {
			t.Fatal(err)
		}

		if asset, err := impostor.Prepared("mismatched-mobile-storage", encoded).Load(app); err == nil || asset != nil {
			t.Fatal("admitted ETC2 blocks under another storage descriptor", storage, err)
		}
	}

	wrong := document
	wrong.Color = wrong.NormalDepth

	encoded, err = impostordata.Encode(wrong)
	if err != nil {
		t.Fatal(err)
	}

	if asset, err := impostor.Prepared("linear-color", encoded).Load(app); err == nil || asset != nil {
		t.Fatal("admitted a linear mobile color atlas", err)
	}
}
