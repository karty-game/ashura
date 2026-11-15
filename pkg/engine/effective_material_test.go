package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type unmappedStandardFixture struct {
	runtimeMaterialFixture

	normalStrength, occlusionStrength float32
}

func (value unmappedStandardFixture) RuntimeTextures() (*texture.Texture, *texture.Texture, Vector2, float32) {
	return nil, nil, Vector2{X: 1, Y: 1}, value.normalStrength
}
func (value unmappedStandardFixture) RuntimeOcclusion() (*texture.Texture, float32) {
	return nil, value.occlusionStrength
}

type effectiveSourceFixture struct {
	runtimeMaterialFixture

	ambient  [3]Color
	strength float32
}

func (value effectiveSourceFixture) RuntimeSource() ([3]Color, bool, float32) {
	return value.ambient, false, value.strength
}

func TestOpaqueUnusedFieldsKeepOneInstanceBatch(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	geometry := instanceFixture{geometryFixture: fixtureTriangle(), source: new(int)}
	first := world.Add(DefaultTransform(), geometry, unmappedStandardFixture{
		color: Color{R: .2, A: 1}, roughness: .5, normalStrength: 1,
	})
	second := world.Add(DefaultTransform(), geometry, unmappedStandardFixture{
		color: Color{R: .2, A: .5}, roughness: .5, normalStrength: 99, occlusionStrength: 88,
	})

	frame := geometryFrame(t, world)
	if len(frame.Draws) != 1 || len(frame.Draws[0].Instances) != 2 {
		t.Fatal("opaque alpha or missing-map strengths split one material into multiple draws")
	}

	var uniforms render.UniformState
	uniforms.Update(frame.Draws[0], frame)

	stored, _ := Get[unmappedStandardFixture](world, first)
	stored.color.A, stored.normalStrength, stored.occlusionStrength = .7, 22, 33

	frame = geometryFrame(t, world)
	if len(frame.Draws) != 1 || uniforms.Update(frame.Draws[0], frame) {
		t.Fatal("shader-unused field mutation split a batch or triggered uniform encoding")
	}

	storedSecond, _ := Get[unmappedStandardFixture](world, second)
	if storedSecond.color.A != .5 || storedSecond.normalStrength != 99 {
		t.Fatal("effective draw key mutated stored component values")
	}

	storedSecond.color.G = .3

	if len(geometryFrame(t, world).Draws) != 2 {
		t.Fatal("meaningful RGB mutation failed to split distinct instance materials")
	}
}

func TestSourceUnusedFieldsKeepOneInstanceBatch(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	geometry := instanceFixture{geometryFixture: fixtureTriangle(), source: new(int)}
	base := effectiveSourceFixture{
		color: Color{R: .2, A: 1}, roughness: .1,
		ambient: [3]Color{{R: .3, A: 1}}, strength: 1,
	}
	first := world.Add(DefaultTransform(), geometry, base)
	base.color.A, base.roughness, base.ambient[0].A, base.strength = .5, .9, .7, 99
	world.Add(DefaultTransform(), geometry, base)

	frame := geometryFrame(t, world)
	if len(frame.Draws) != 1 || len(frame.Draws[0].Instances) != 2 {
		t.Fatal("Source alpha or unused Standard/no-map fields split instances")
	}

	var uniforms render.UniformState
	uniforms.Update(frame.Draws[0], frame)

	stored, _ := Get[effectiveSourceFixture](world, first)
	stored.color.A, stored.ambient[0].A, stored.roughness, stored.strength = .9, .6, .4, 33

	frame = geometryFrame(t, world)
	if len(frame.Draws) != 1 || uniforms.Update(frame.Draws[0], frame) {
		t.Fatal("Source shader-unused fields changed the effective material")
	}
}
