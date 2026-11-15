//go:build !js

package model

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
)

type testLightmapLayout struct {
	digest [32]byte
	uv     []byte
}

func (l testLightmapLayout) ModelDigest() [32]byte { return l.digest }
func (l testLightmapLayout) PartCount() int        { return 1 }
func (l testLightmapLayout) PartUV(int) []byte     { return l.uv }

func TestLightmapLoadBindsExactModelAndOwnsUVs(t *testing.T) {
	t.Parallel()
	asset, data := fixtureAsset(t, false)

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	layout := testLightmapLayout{digest: sha256.Sum256(data), uv: make([]byte, 24)}
	for index, value := range []float32{0, 0, 1, 0, 0, 1} {
		binary.LittleEndian.PutUint32(layout.uv[index*4:], math.Float32bits(value))
	}

	loaded, err := asset.LoadLightmapped(app, layout)
	if err != nil {
		t.Fatal(err)
	}

	uv := loaded.parts[0].geometry.LightmapUV()

	layout.uv[0] = 255
	if uv[0] != 0 {
		t.Fatal("caller mutated admitted UV stream")
	}

	layout.digest[0] ^= 1
	if _, err = asset.LoadLightmapped(app, layout); err == nil {
		t.Fatal("accepted UVs for different prepared model")
	}

	layout.digest = sha256.Sum256(data)

	layout.uv = layout.uv[:8]
	if _, err = asset.LoadLightmapped(app, layout); err == nil {
		t.Fatal("accepted incomplete UV stream")
	}
}
