//go:build !js

package level

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/pkg/decal"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestChunkPresentationOwnsPreparedDecals(t *testing.T) {
	t.Parallel()

	app, _, chunk := loadPresentationChunk(t, 2)
	world := engine.NewWorld()

	batchBytes, err := decaldata.Encode([]decaldata.Placement{{
		Normal: decaldata.Vector{0, 1, 0}, Size: decaldata.Vector{1, 1, 1},
		Max: [2]float32{1, 1}, Tint: [4]float32{1, 1, 1, 1}, Layers: 2,
	}})
	if err != nil {
		t.Fatal(err)
	}

	batch, err := decal.DecodePrepared(batchBytes)
	if err != nil {
		t.Fatal(err)
	}

	atlas, err := app.NewTexture(texture.Data{Width: 1, Height: 1, ColorSpace: texture.SRGB, Levels: [][]byte{{255, 0, 0, 255}}})
	if err != nil {
		t.Fatal(err)
	}

	instance, err := chunk.BeginPresentation(world)
	if err != nil {
		t.Fatal(err)
	}

	marks, err := instance.SetDecals(ChunkDecals{Batch: batch, Atlas: atlas, ReceiverLayers: 2})
	if err != nil {
		t.Fatal(err)
	}

	if done, err := instance.PresentNext(); !done || err != nil {
		t.Fatalf("chunk presentation: done=%t error=%v", done, err)
	}

	count := 0

	for _, receiver := range engine.Query[decal.Receiver](world) {
		if receiver.Layers != 2 {
			t.Fatal("unexpected decal receiver mask", receiver.Layers)
		}

		count++
	}

	if count != 2 {
		t.Fatal("chunk did not opt all material ranges into decals", count)
	}

	if err := marks.SetEnabled(0, false); err != nil {
		t.Fatal(err)
	}

	instance.Remove()

	if err := marks.SetEnabled(0, true); !errors.Is(err, engine.ErrClosed) {
		t.Fatal("removing a chunk retained its projector", err)
	}
}
