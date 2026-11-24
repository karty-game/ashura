//go:build !js

package model

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/resourcedata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/scene/resource"
)

func TestVerifiedModelRetainsImmutableReaderGeometryAndExactBinding(t *testing.T) {
	t.Parallel()
	_, data := fixtureAsset(t, false)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	digest := sha256.Sum256(data)

	payload, err := resource.ReadVerified(
		t.Context(),
		root,
		resource.Asset{ID: "model", Path: "model.bin", Size: len(data), SHA256: hex.EncodeToString(digest[:])},
	)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	layout := testLightmapLayout{digest: digest, uv: make([]byte, 24)}

	loaded, err := Verified("model", payload).LoadLightmapped(app, layout)
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(resourcedata.Bytes(payload))
	if err != nil {
		t.Fatal(err)
	}

	if &loaded.parts[0].geometry.Vertices()[0] != &document.Parts[0].Vertices[0] ||
		&loaded.parts[0].geometry.Indices()[0] != &document.Parts[0].Indices[0] {
		t.Fatal("verified immutable geometry was copied again")
	}

	layout.uv[0] = 255

	if loaded.parts[0].geometry.LightmapUV()[0] != 0 {
		t.Fatal("verified geometry retained caller-owned UV bytes")
	}

	layout.digest[0] ^= 1
	if _, err := Verified("model", payload).LoadLightmapped(app, layout); err == nil {
		t.Fatal("verified geometry admitted a different model's UV layout")
	}

	if _, err := Verified("unverified", &resource.Payload{}).Load(app); err == nil {
		t.Fatal("zero payload bypassed resource integrity admission")
	}

	second, err := Verified("model", payload).Load(app)
	if err != nil {
		t.Fatal(err)
	}

	if err := loaded.Unload(); err != nil || len(second.parts[0].geometry.Vertices()) == 0 {
		t.Fatal("unloading one model invalidated another immutable consumer", err)
	}
}
