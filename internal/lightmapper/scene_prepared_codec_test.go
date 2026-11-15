package lightmapper

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func testPreparedGeometry(t *testing.T) *PreparedScene {
	t.Helper()

	source := testCoreScene()
	source.Materials = []Material{{ID: "surface", Reflectance: Vec3{.5, .5, .5}}}

	for i := range 16 {
		x := float32(i) * 2
		source.Triangles = append(
			source.Triangles,
			Triangle{ID: uint64(i + 1), Material: 0, Positions: [3]Vec3{{x, 0, 0}, {x + 1, 0, 0}, {x, 1, 0}}},
		)
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	return prepared
}

func TestPreparedSceneArtifactPreservesAccelerationAndRejectsForgedCycle(t *testing.T) {
	t.Parallel()
	prepared := testPreparedGeometry(t)
	// Valid wider bounds intentionally differ from the SAH builder. A cached
	// decode must preserve these exact stored nodes rather than rebuilding them.
	prepared.nodes[0].Min[0] = -100

	prepared.geometryHash = ""
	if err := prepared.finishSceneIdentity(); err != nil {
		t.Fatal(err)
	}

	encoded, err := EncodeScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeScene(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Nodes()[0].Min[0] != -100 || decoded.Hash() != prepared.Hash() || decoded.GeometryHash() != prepared.GeometryHash() {
		t.Fatal("cached acceleration was replaced")
	}

	sourceSize := int(binary.LittleEndian.Uint32(encoded[44:]))
	nodeStart := 44 + 4 + sourceSize + 4
	binary.LittleEndian.PutUint32(encoded[nodeStart+24:], 0)
	digest := sha256.Sum256(encoded[44:])
	copy(encoded[12:44], digest[:])

	if _, err = DecodeScene(encoded); err == nil {
		t.Fatal("checksummed BVH cycle admitted")
	}
}

func TestWithLightingSharesGeometryAndInvalidatesOnlyLightingIdentity(t *testing.T) {
	t.Parallel()
	base := testPreparedGeometry(t)
	emitters := []Emitter{{ID: 20, Kind: Point, Position: Vec3{0, 0, 2}, Intensity: Vec3{1, 1, 1}}}

	changed, err := WithLighting(base, emitters, Environment{Sky: Vec3{.1, .1, .1}})
	if err != nil {
		t.Fatal(err)
	}

	if &changed.nodes[0] != &base.nodes[0] || &changed.order[0] != &base.order[0] ||
		&changed.source.Triangles[0] != &base.source.Triangles[0] ||
		&changed.source.Materials[0] != &base.source.Materials[0] {
		t.Fatal("lighting change rebuilt or cloned geometry")
	}

	if changed.GeometryHash() != base.GeometryHash() || changed.Hash() == base.Hash() {
		t.Fatal("cache identity did not separate geometry and light")
	}

	emitters[0].Intensity = Vec3{3, 3, 3}
	if changed.Source().Emitters[0].Intensity != (Vec3{1, 1, 1}) {
		t.Fatal("lighting input retained caller storage")
	}

	encoded, err := EncodeScene(changed)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeScene(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.GeometryHash() != base.GeometryHash() || decoded.Hash() != changed.Hash() {
		t.Fatal("cached relighting changed identity")
	}
}

func TestReferenceStableVarianceDoesNotRequireOverflowingRawSecondMoment(t *testing.T) {
	t.Parallel()

	var moment referenceWelford
	for i := range uint32(16) {
		if err := moment.add(1e20, i+1); err != nil {
			t.Fatal(err)
		}
	}

	mean, variance, raw, rawValid, err := moment.finish(16)
	if err != nil || mean != float32(1e20) || variance != 0 || rawValid || raw != 0 {
		t.Fatal(mean, variance, raw, rawValid, err)
	}
}
