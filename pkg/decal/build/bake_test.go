//go:build !js

package decalbuild

import (
	"bytes"
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/engine"
)

func TestBakeAlbedoMasksProjectionAndKeepsReceiverCoverage(t *testing.T) {
	t.Parallel()

	front := Triangle{
		Positions: [3]engine.Vector3{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: -1, Y: 1}},
		UVs:       [3]engine.Vector2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}},
	}

	projector, err := (decaldata.Placement{
		Position: decaldata.Vector{-.6, -.6, 0}, Normal: decaldata.Vector{0, 0, 1},
		Size: decaldata.Vector{.7, .7, .25}, Max: [2]float32{1, 1}, Tint: [4]float32{1, 1, 1, 1}, Layers: 1,
	}).Project()
	if err != nil {
		t.Fatal(err)
	}

	white := []byte{0, 0, 0, 121}
	receiver := assetbuild.Image{Width: 8, Height: 8, Levels: [][]byte{bytes.Repeat(white, 64)}}
	source := assetbuild.Image{Width: 1, Height: 1, Levels: [][]byte{{255, 0, 0, 128}}}

	marks := []bakedMark{{source, projector}}
	if err := paintAlbedo(t.Context(), &receiver, []Triangle{front}, marks); err != nil {
		t.Fatal(err)
	}

	marked := (8 + 1) * 4
	if got := receiver.Levels[0][marked : marked+4]; got[0] != 188 || got[1] != 0 || got[3] != 121 {
		t.Fatal("wrong linear blend or receiver coverage", got)
	}

	if got := receiver.Levels[0][16:20]; !bytes.Equal(got, white) {
		t.Fatal("unprojected texel changed", got)
	}

	if err := paintAlbedo(t.Context(), &receiver, []Triangle{front, front}, marks); !errors.Is(err, errReceiverUV) {
		t.Fatal("overlapping UVs should fail", err)
	}
}
