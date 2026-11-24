package engine

import (
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
)

type impostorQualityFixture struct {
	runtimeMaterialFixture

	document *impostordata.Document
	quality  [4]float32
}

func (value *impostorQualityFixture) RuntimeImpostor() *impostordata.Document { return value.document }
func (value *impostorQualityFixture) RuntimeImpostorQuality() [4]float32      { return value.quality }

func TestSharedImpostorQualityEditsRefreshBindingsAndBatches(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	geometry := instanceFixture{geometryFixture: fixtureTriangle(), source: new(int)}
	document := &impostordata.Document{Mode: impostordata.ModeOctahedral}
	first := &impostorQualityFixture{document: document, quality: [4]float32{16, 128, 2, 0}}
	second := &impostorQualityFixture{document: document, quality: first.quality}
	world.Add(DefaultTransform(), geometry, first)
	secondEntity := world.Add(DefaultTransform(), geometry, second)

	second, _ = Get[impostorQualityFixture](world, secondEntity)
	if frame := geometryFrame(t, world); len(frame.Draws) != 1 || frame.Draws[0].ImpostorQuality != first.quality {
		t.Fatal("matching settings lost instancing or cached provider values")
	}

	second.quality[2] = -2

	frame := geometryFrame(t, world)
	if len(frame.Draws) != 2 || frame.Draws[0].ImpostorQuality == frame.Draws[1].ImpostorQuality {
		t.Fatal("quality edit was not visible without component rebinding")
	}

	second.quality = first.quality

	if frame := geometryFrame(t, world); len(frame.Draws) != 1 {
		t.Fatal("restored matching settings failed to rejoin the batch")
	}
}
