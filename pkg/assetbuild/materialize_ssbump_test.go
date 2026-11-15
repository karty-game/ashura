//go:build !js

package assetbuild

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func TestMaterializeAndSSBumpComposeAndReuseUnchangedMaps(t *testing.T) {
	t.Parallel()
	project, config, options, material := materializeFixture(t)

	config.Plugins = []Plugin{
		Images(), Materialize(material),
		SSBump(SSBumpOptions{Include: []string{"*-normal"}, Rays: 8, Distance: 2}),
		Mipmaps(), KTX2(), GoCatalog(),
	}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	assertMaterializeCalls(t, material, 1)

	encoded, err := fixtureRead(project, "generated/assets/brick-ssbump.ktx2")
	if err != nil {
		t.Fatal(err)
	}

	data, err := ktx2.Decode(encoded)
	if err != nil || data.ColorSpace != texture.Linear || data.Width != 4 || len(data.Levels) != 3 {
		t.Fatalf("prepared SSBump texture: %+v %v", data, err)
	}

	if _, err := fixtureRead(project, "generated/assets/brick-normal.ktx2"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replaced normal remained published: %v", err)
	}

	code, err := fixtureRead(project, "generated/assets/assets.go")
	if err != nil || !bytes.Contains(code, []byte("func BrickSSBump()")) || !bytes.Contains(code, []byte("func BrickAO()")) {
		t.Fatalf("composed catalog: %s %v", code, err)
	}

	warm, err := Build(t.Context(), config, options)
	if err != nil || warm.Executed != 0 {
		t.Fatalf("warm composition: %+v %v", warm, err)
	}

	strength := 2.0
	material.NormalStrength = &strength
	config.Plugins[1] = Materialize(material)

	changed, err := Build(t.Context(), config, options)
	if err != nil || changed.Executed != 1 {
		t.Fatalf("identical generated maps must stop invalidation before baking: %+v %v", changed, err)
	}

	assertMaterializeCalls(t, material, 2)
}
