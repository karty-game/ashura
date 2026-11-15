package lightmapper

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"testing"
)

func TestGlobalLightProposalUsesIncomingEnergyAndFiniteBounds(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Emitters = []Emitter{
		{ID: 1, Kind: Point, Position: Vec3{0, 0, 2}, Intensity: Vec3{1, 1, 1}},
		{ID: 2, Kind: Directional, Direction: Vec3{0, 0, -1}, Intensity: Vec3{100, 100, 100}},
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	globalMass := prepared.LightPMF(Vec3{}, 1)
	if globalMass < .9 || globalMass >= 1 {
		t.Fatal("dominant global source starved by artificial spatial bounds", globalMass)
	}

	root := prepared.LightNodes()[0]
	if root.Min != (Vec3{0, 0, 2}) || root.Max != root.Min || root.GlobalPower != 100 {
		t.Fatal("global sources polluted finite bounds", root)
	}

	encoded, err := EncodeScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeScene(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Hash() != prepared.Hash() || decoded.LightPMF(Vec3{}, 1) != globalMass {
		t.Fatal("cached global light proposal changed")
	}

	mass := float64(prepared.LightPMF(Vec3{}, 0)) + float64(globalMass)
	if math.Abs(mass-1) > 1e-6 {
		t.Fatal("mixed global/finite probability mass", mass)
	}
}

func TestCachedLightClassificationCannotHideFiniteSourceBounds(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Emitters = []Emitter{
		{ID: 1, Kind: Rectangle, Direction: Vec3{0, 0, 1}, Tangent: Vec3{1, 0, 0}, Size: Vec2{1, 1}, Intensity: Vec3{1, 1, 1}},
		{ID: 2, Kind: Directional, Direction: Vec3{0, 0, -1}, Intensity: Vec3{100, 100, 100}},
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := EncodeScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	// This fixture has no geometry nodes/order. The three count words following
	// its source payload frame the two empty arenas and the light-node arena.
	sourceBytes := int(binary.LittleEndian.Uint32(encoded[44:]))
	firstNode := 48 + sourceBytes + 12

	for emitter := range source.Emitters {
		forged := slices.Clone(encoded)
		leaf := int(prepared.lightLeaves[emitter])
		offset := firstNode + leaf*binary.Size(LightNode{})

		finitePower, globalPower := float32(0), float32(1)
		if globalEmitter(source.Emitters[emitter].Kind) {
			finitePower, globalPower = 1, 0
		}

		binary.LittleEndian.PutUint32(forged[offset+40:], math.Float32bits(finitePower))
		binary.LittleEndian.PutUint32(forged[offset+60:], math.Float32bits(globalPower))
		digest := sha256.Sum256(forged[44:])
		copy(forged[12:44], digest[:])

		if _, err := DecodeScene(forged); !errors.Is(err, ErrInvalid) {
			t.Fatal("checksummed cache changed emitter spatial classification", emitter, err)
		}
	}
}
