package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

type alphaMaskFixture struct {
	runtimeMaterialFixture

	enabled bool
	cutoff  float32
}

func (value alphaMaskFixture) RuntimeAlphaMask() (bool, float32) { return value.enabled, value.cutoff }

func TestAlphaMaskProviderPreservesZeroAndBatchIdentity(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	geometry := instanceFixture{geometryFixture: fixtureTriangle(), source: new(int)}
	masked := alphaMaskFixture{color: Color{R: .2, A: 0}, roughness: .5, enabled: true, cutoff: .5}
	first := world.Add(DefaultTransform(), geometry, masked)
	masked.color.A = 1
	world.Add(DefaultTransform(), geometry, masked)

	frame := geometryFrame(t, world)
	if len(frame.Draws) != 2 || !frame.Draws[0].AlphaMask || frame.Draws[0].Color[3] != 0 || frame.Draws[0].AlphaCutoff != .5 {
		t.Fatal("cutout alpha zero was defaulted away or distinct coverage was batched")
	}

	stored, _ := Get[alphaMaskFixture](world, first)
	stored.color.A = 1

	frame = geometryFrame(t, world)
	if len(frame.Draws) != 1 || len(frame.Draws[0].Instances) != 2 {
		t.Fatal("identical cutouts failed to instance")
	}

	stored.cutoff = 0

	frame = geometryFrame(t, world)
	if len(frame.Draws) != 2 || frame.Draws[0].AlphaCutoff != 0 || !frame.Draws[0].AlphaMask {
		t.Fatal("zero cutoff lost or cutoff mutation failed to split the batch")
	}

	if err := world.Set(first, runtimeMaterialFixture{color: Color{R: .2, A: 0}, roughness: .5}); err != nil {
		t.Fatal(err)
	}

	frame = geometryFrame(t, world)
	if frame.Draws[0].AlphaMask || frame.Draws[0].AlphaCutoff != 0 || frame.Draws[0].Color[3] != 1 {
		t.Fatal("opaque provider retained cutout state or changed its alpha default")
	}

	if !RemoveComponent[runtimeMaterialFixture](world, first) {
		t.Fatal("active opaque provider removal failed")
	}

	frame = geometryFrame(t, world)
	if !frame.Draws[0].AlphaMask || frame.Draws[0].AlphaCutoff != 0 {
		t.Fatal("provider fallback lost retained cutout component")
	}
}

func TestFrameAdmissionRejectsUnsupportedAlphaMask(t *testing.T) {
	t.Parallel()

	frame := render.Frame{Draws: []render.DrawCall{{
		Source: true, AlphaMask: true, AlphaCutoff: .5, Color: [4]float32{1, 1, 1, 1},
	}}}
	if err := validateFrame(frame); err == nil {
		t.Fatal("frame silently accepted unsupported Source cutout")
	}

	world := NewWorld()
	world.Add(DefaultTransform(), fixtureTriangle(), alphaMaskFixture{color: Color{A: 1}, enabled: true, cutoff: 2})

	if _, err := makeFrame(world, 100, 100); err == nil {
		t.Fatal("invalid cutout cutoff reached a render frame")
	}
}
