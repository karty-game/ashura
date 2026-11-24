//go:build !js

package model

import (
	"crypto/sha256"
	"testing"

	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
)

func TestMaskedPreparedModelBindsCoverageAndExactModelProvenance(t *testing.T) {
	t.Parallel()

	_, opaque := fixtureAsset(t, false)

	document, err := modeldata.Decode(opaque)
	if err != nil {
		t.Fatal(err)
	}

	document.Parts[0].Material.AlphaMask = true
	document.Parts[0].Material.AlphaCutoff = .35
	document.Parts[0].Material.Color[3] = .75

	masked, err := modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	asset := Prepared("masked.glb", masked)

	layout := testLightmapLayout{digest: sha256.Sum256(opaque), uv: make([]byte, 24)}
	if _, err := asset.LoadLightmapped(app, layout); err == nil {
		t.Fatal("v2 cutout accepted UV provenance from different opaque bytes")
	}

	loaded, err := asset.Load(app)
	if err != nil {
		t.Fatal(err)
	}

	world := engine.NewWorld()

	instance, err := loaded.Instantiate(world, engine.DefaultTransform())
	if err != nil {
		t.Fatal(err)
	}

	bound, exists := engine.Get[material.Material](world, instance.Entities()[0])
	if !exists || !bound.AlphaMask || bound.AlphaCutoff != .35 || bound.Color.A != .75 {
		t.Fatal("prepared coverage was lost when binding Standard", bound)
	}

	instance.Remove()

	if err := loaded.Unload(); err != nil {
		t.Fatal(err)
	}
}
