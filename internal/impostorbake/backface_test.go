//go:build !js

package impostorbake

import (
	"encoding/binary"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestDoubleSidedCaptureFlipsCompleteMappedNormal(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		mapped   bool
		strength float32
	}{
		{name: "geometric"},
		{name: "tangent-normal", mapped: true, strength: 1},
		{name: "disabled-tangent-normal", mapped: true, strength: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := fixtureDocument()
			source.Parts = source.Parts[:1]
			source.Parts[0].Material.DoubleSided = true

			if test.mapped {
				data, err := assetbuild.EncodeTexture(assetbuild.Image{Name: "Normal", Source: "normal.png", Usage: assetbuild.UsageNormal,
					Width: 1, Height: 1, Levels: [][]byte{{255, 128, 128, 255}}})
				if err != nil {
					t.Fatal(err)
				}

				source.Textures = []modeldata.Texture{{Role: modeldata.Normal, Data: data}}
				source.Parts[0].Material.Normal = 0
				source.Parts[0].Material.NormalStrength = test.strength
			}

			baked, err := Bake(t.Context(), source, Options{Resolution: 32, Azimuths: 4, Elevations: 3, Padding: 2, MipLevels: 2})
			if err != nil {
				t.Fatal(err)
			}

			axis := 2
			if test.mapped && test.strength != 0 {
				axis = 0
			}

			checkOppositeNormals(t, baked, axis)
		})
	}
}

func checkOppositeNormals(t *testing.T, baked impostordata.Document, axis int) {
	t.Helper()

	front, back := viewTexel(t, baked.NormalDepth, baked, 4), viewTexel(t, baked.NormalDepth, baked, 6)
	frontColor, backColor := viewTexel(t, baked.Color, baked, 4), viewTexel(t, baked.Color, baked, 6)

	if frontColor[3] != 255 || backColor[3] != 255 {
		t.Fatalf("double-sided face missing from capture: front%v back%v", frontColor, backColor)
	}

	if front[axis] < 250 || back[axis] > 5 {
		t.Fatalf("backside normal did not flip complete mapped direction: front%v back%v", front, back)
	}

	for channel := range 3 {
		sum := int(front[channel]) + int(back[channel])
		if sum < 254 || sum > 256 {
			t.Fatalf("normal channel%d is not opposite: front%v back%v", channel, front, back)
		}
	}
}

//nolint:gosec // The fixture reads offsets from its freshly encoded bounded KTX2 payload.
func viewTexel(t *testing.T, data []byte, document impostordata.Document, view int) [4]byte {
	t.Helper()

	offset := binary.LittleEndian.Uint64(data[80:])
	column := (view%int(document.Columns))*int(document.CellSize) + int(document.Padding) + int(document.Resolution)/2
	row := (view/int(document.Columns))*int(document.CellSize) + int(document.Padding) + int(document.Resolution)/2
	index := int(offset) + (row*int(document.AtlasWidth)+column)*4

	return [4]byte(data[index : index+4])
}
