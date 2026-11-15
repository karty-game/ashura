package lightmapper

import (
	"math"
	"testing"
)

func testCoreScene() Scene { return Scene{ID: "fixture", Space: "room", MetresPerUnit: 1} }

func TestLightHierarchyMassAndSplitSourceEnergy(t *testing.T) {
	t.Parallel()

	source := testCoreScene()

	const count = 16384

	source.Emitters = make([]Emitter, count)
	for emitterIndex := range source.Emitters {
		source.Emitters[emitterIndex] = Emitter{
			ID:        uint64(emitterIndex + 1),
			Kind:      Point,
			Position:  Vec3{0, 0, 2},
			Intensity: Vec3{4.0 / count, 2.0 / count, 1.0 / count},
		}
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	var mass float64

	seen := make([]bool, count)
	for emitterIndex := range count {
		lightIndex, pdf := prepared.SampleLight(Vec3{}, (float32(emitterIndex)+.5)/count)
		if seen[lightIndex] || pdf <= 0 || pdf != prepared.LightPMF(Vec3{}, lightIndex) {
			t.Fatal(lightIndex, pdf)
		}

		seen[lightIndex] = true
		mass += float64(prepared.LightPMF(Vec3{}, uint32(emitterIndex)))

		v := prepared.SampleEmitter(Vec3{}, lightIndex, .25, .5)
		for c, want := range (Vec3{1, .5, .25}) {
			if math.Abs(float64(v.Radiance[c]/pdf-want)) > 1e-6 {
				t.Fatal("split source energy", v, pdf)
			}
		}
	}

	if math.Abs(mass-1) > 1e-6 {
		t.Fatal("mass", mass)
	}
}

func TestSpatialLightHierarchyHasRecoverableUnequalMass(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	for emitterIndex := range 64 {
		source.Emitters = append(
			source.Emitters,
			Emitter{ID: uint64(emitterIndex + 1), Kind: Point, Position: Vec3{float32(emitterIndex), 0, 1}, Intensity: Vec3{1, 1, 1}},
		)
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	sum := float64(0)
	for emitterIndex := range source.Emitters {
		sum += float64(prepared.LightPMF(Vec3{}, uint32(emitterIndex)))
	}

	if math.Abs(sum-1) > 1e-5 {
		t.Fatal(sum)
	}

	if prepared.LightPMF(Vec3{}, 0) <= prepared.LightPMF(Vec3{}, 63) {
		t.Fatal("distance importance missing")
	}

	for emitterIndex := range 4096 {
		lightIndex, mass := prepared.SampleLight(Vec3{}, (float32(emitterIndex)+.5)/4096)
		if math.Abs(float64(mass-prepared.LightPMF(Vec3{}, lightIndex))) > 1e-6 {
			t.Fatal(lightIndex, mass)
		}
	}
}

func TestPreparedSceneCodecIntegrityAndOwnership(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Environment = Environment{Sky: Vec3{.2, .2, .2}, Ground: Vec3{.2, .2, .2}}
	source.Textures = []Texture{{Width: 1, Height: 1, Pixels: []Vec3{{.2, .3, .4}}}}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	source.Textures[0].Pixels[0] = Vec3{1, 1, 1}
	if prepared.Source().Textures[0].Pixels[0] != (Vec3{.2, .3, .4}) {
		t.Fatal("borrowed source")
	}

	encoded, err := EncodeScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeScene(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Hash() != prepared.Hash() || len(decoded.Source().Emitters) != 1 {
		t.Fatal("roundtrip identity", decoded.Hash(), prepared.Hash())
	}

	encoded[len(encoded)-1] ^= 1
	if _, err = DecodeScene(encoded); err == nil {
		t.Fatal("corrupt scene admitted")
	}
}

func TestReflectanceBilinearLinearAndNotReceiverAlbedo(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}, Texture: 1}}
	source.Textures = []Texture{{Width: 2, Height: 1, Pixels: []Vec3{{0, 0, 0}, {1, .5, 0}}}}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	if got := prepared.Reflectance(0, Vec2{.5, .5}); got != (Vec3{.25, .125, 0}) {
		t.Fatal(got)
	}
}

func TestEnvironmentHDRCDFRetainsSupportAndMatchingPDF(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Textures = []Texture{{Width: 64, Height: 32, Pixels: make([]Vec3, 64*32)}}
	source.Textures[0].Pixels[10] = Vec3{1e20, 1e20, 1e20}
	source.Environment = Environment{Texture: 1}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	for _, cdf := range [][]float32{prepared.EnvironmentRows(), prepared.EnvironmentColumns()[:65]} {
		for emitterIndex := 1; emitterIndex < len(cdf); emitterIndex++ {
			if cdf[emitterIndex] <= cdf[emitterIndex-1] {
				t.Fatal("CDF lost support", emitterIndex)
			}
		}
	}

	for emitterIndex := range 64 {
		direction, pdf := prepared.SampleEnvironment((float32(emitterIndex)+.37)/64, .723)

		actual := prepared.EnvironmentPDF(direction)
		if pdf <= 0 || math.Abs(float64(pdf-actual)) > float64(pdf)*.002 {
			t.Fatal("conditional density mismatch", direction, pdf, actual)
		}
	}
}

func TestEnvironmentLatitudeClampsAtPoles(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Textures = []Texture{{Width: 1, Height: 2, Pixels: []Vec3{{1, 0, 0}, {0, 0, 1}}}}
	source.Environment = Environment{Texture: 1}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	if prepared.EnvironmentRadiance(Vec3{0, 1, 0}) != (Vec3{1, 0, 0}) || prepared.EnvironmentRadiance(Vec3{0, -1, 0}) != (Vec3{0, 0, 1}) {
		t.Fatal("environment wrapped latitude")
	}
}

func TestDirectionalLightHierarchyRetainsEveryContributor(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Emitters = []Emitter{
		{ID: 1, Kind: Spot, Position: Vec3{0, 0, 2}, Direction: Vec3{0, 0, -1}, Intensity: Vec3{1, 1, 1}, InnerCos: .9, OuterCos: .8},
		{ID: 2, Kind: Spot, Position: Vec3{0, 0, 2}, Direction: Vec3{0, 0, 1}, Intensity: Vec3{1, 1, 1}, InnerCos: .9, OuterCos: .8},
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	toward, away := prepared.LightPMF(Vec3{}, 0), prepared.LightPMF(Vec3{}, 1)
	if toward <= away || away <= 0 || math.Abs(float64(toward+away-1)) > 1e-6 {
		t.Fatal(toward, away)
	}

	for emitterIndex := range 256 {
		id, mass := prepared.SampleLight(Vec3{}, (float32(emitterIndex)+.5)/256)
		if math.Abs(float64(mass-prepared.LightPMF(Vec3{}, id))) > 1e-6 {
			t.Fatal(id, mass)
		}
	}
}

func TestEnvironmentSamplingPreservesPolarAngularResolution(t *testing.T) {
	t.Parallel()

	source := testCoreScene()

	source.Textures = []Texture{{Width: 1, Height: 8192, Pixels: make([]Vec3, 8192)}}
	for emitterIndex := range source.Textures[0].Pixels {
		source.Textures[0].Pixels[emitterIndex] = Vec3{1, 1, 1}
	}

	source.Environment = Environment{Texture: 1}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	for _, u := range []float32{prepared.environmentRows[1] * .2, 1 - (1-prepared.environmentRows[8191])*.2} {
		direction, pdf := prepared.SampleEnvironment(u, .25)

		actual := prepared.EnvironmentPDF(direction)
		if direction[0] == 0 && direction[2] == 0 || pdf <= 0 || math.Abs(float64(pdf-actual)) > float64(pdf)*.002 {
			t.Fatal(direction, pdf, actual)
		}
	}
}

func TestImplicitEmitterIdentityCollisionResolutionIsStable(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}, Emission: Vec3{1, 1, 1}}}
	source.Triangles = []Triangle{{ID: 1, Positions: [3]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, Material: 0}}
	source.Environment = Environment{Sky: Vec3{.1, .1, .1}}
	source.Emitters = []Emitter{
		{ID: 0xf000000000000001, Kind: Point, Position: Vec3{0, 0, 1}, Intensity: Vec3{1, 1, 1}},
		{ID: math.MaxUint64, Kind: Point, Position: Vec3{1, 0, 1}, Intensity: Vec3{1, 1, 1}},
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	ids := make(map[uint64]bool)
	for _, e := range prepared.Source().Emitters {
		if ids[e.ID] {
			t.Fatal("duplicate implicit identity", e.ID)
		}

		ids[e.ID] = true
	}

	encoded, err := EncodeScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeScene(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Hash() != prepared.Hash() {
		t.Fatal("implicit identity changed on cache read")
	}
}

func TestLinearRepeatSamplerWrapsBeforeIntegerConversion(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{Reflectance: Vec3{1, 1, 1}, Texture: 1}}
	source.Textures = []Texture{{Width: 2, Height: 2, Pixels: []Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 1, 1}}}}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	for _, coordinates := range []Vec2{{1e20, -1e20}, {-1e30, 1e30}, {1e20, .25}} {
		value := prepared.Reflectance(0, coordinates)
		wrapped := Vec2{
			coordinates[0] - float32(math.Floor(float64(coordinates[0]))),
			coordinates[1] - float32(math.Floor(float64(coordinates[1]))),
		}

		expected := prepared.Reflectance(0, wrapped)
		if !validVec(value) || value != expected {
			t.Fatal("finite repeated UV overflowed", coordinates, value, expected)
		}
	}
}
