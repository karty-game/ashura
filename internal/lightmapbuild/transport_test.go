//go:build !js

package lightmapbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
	modelbuild "github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
)

func TestPrecomputedTransportIsDeterministicAndPreservesDirectionalEnergy(t *testing.T) {
	t.Parallel()

	var first []byte

	for attempt := range 2 {
		document := lightmapfixture.RNMDocument()
		triangles := []triangle{{points: [3]vec{{-2, 1, -2}, {0, 1, -2}, {0, 1, 2}}}}

		geometric := []vec{{0, 1, 0}, {0, 1, 0}, {}, {}}
		if err := buildTransport(t.Context(), &document, triangles, geometric, 64); err != nil {
			t.Fatal(err)
		}

		if !document.Transport() || document.GISamples != 64 || len(document.GIReceivers) != 64 || len(document.GITransfers) == 0 {
			t.Fatal("missing prepared receiver transport")
		}

		if len(document.GIPatches) == 0 {
			t.Fatal("visible reflector did not become physical patch")
		}

		for pixel := range 4 {
			index := binary.LittleEndian.Uint32(document.ReceiverGIIndex[pixel*4:])
			if pixel < 2 && index != uint32(pixel) || pixel >= 2 && index != math.MaxUint32 {
				t.Fatal("incomplete compact receiver mapping")
			}
		}

		encoded, err := lightmapdata.Encode(document)
		if err != nil {
			t.Fatal(err)
		}

		loaded, err := lightmapdata.Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}

		if !loaded.Transport() || loaded.GISamples != 64 || len(loaded.GPUData) == 0 {
			t.Fatal("lost prepared transport at admission")
		}

		if attempt == 0 {
			first = encoded
		} else if !bytes.Equal(first, encoded) {
			t.Fatal("parallel precompute is nondeterministic")
		}
	}
}

func TestTransportEnvironmentIsNormalizedAndRespectsVisibility(t *testing.T) {
	t.Parallel()

	directions := transportDirections(128)
	empty := transportScene{
		nodes:     []bvhNode{{minimum: vec{-1, -1, -1}, maximum: vec{1, 1, 1}, count: 1}},
		triangles: []transportTriangle{{origin: vec{0, -10, 0}, first: vec{1, 0, 0}, second: vec{0, 0, 1}}},
	}

	sky, ground := patchEnvironment(empty, vec{}, vec{0, 1, 0}, directions)
	if math.Abs(float64(sky+ground-1)) > 1e-6 || math.Abs(float64(sky-5.0/6)) > .005 {
		t.Fatalf("unnormalized open-hemisphere irradiance %f %f", sky, ground)
	}

	box := bounds{minimum: vec{0, 0, 0}, maximum: vec{1, 1, 1}}
	if !transportBox(box, vec{0, .5, -1}, vec{float32(math.Inf(1)), float32(math.Inf(1)), 1}, 10) {
		t.Fatal("parallel ray on slab boundary was incorrectly rejected")
	}
}

func TestPrecomputedTransportCancellation(t *testing.T) {
	t.Parallel()

	document := lightmapfixture.RNMDocument()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := buildTransport(
		ctx,
		&document,
		[]triangle{{points: [3]vec{{-2, 1, -2}, {0, 1, -2}, {0, 1, 2}}}},
		[]vec{{0, 1, 0}, {0, 1, 0}, {}, {}},
		64,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("precompute ignored cancellation: %v", err)
	}
}

func TestPreparedGLBTransportSurvivesRoundTrip(t *testing.T) {
	t.Parallel()

	input, err := modelbuild.Compile(testfixture.Scene(), "transport.glb")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	result, err := prepareFixture(
		ctx,
		input.Data,
		Options{
			Resolution:               64,
			Padding:                  3,
			RNM:                      true,
			PrecomputeTransport:      true,
			PreserveGeometricNormals: true,
			TransportSamples:         16,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	document, err := lightmapdata.Decode(result.Lightmap)
	if err != nil {
		t.Fatal(err)
	}

	if !document.Transport() || document.GISamples != 16 || len(document.GIReceivers)/32 != result.CoveredTexels {
		t.Fatal("transport options did not reach prepared GLB")
	}

	if len(result.GeometricNormals) != result.Width*result.Height*16 {
		t.Fatal("single-atlas preparation lost geometric normal guides")
	}
}

func TestTransportPruningPreservesEveryRNMLobe(t *testing.T) {
	t.Parallel()

	groups := make([]transportGroup, 12)
	total := vec{}

	for index := range groups {
		groups[index].weight = vec{.1, 0, .02}
		if index >= 8 {
			groups[index].weight = vec{0, .001, 0}
		}

		total = add(total, groups[index].weight)
	}

	result := transportReceiver{}
	retainTransportLinks(&result, groups, total)

	if result.count != 8 {
		t.Fatal("did not enforce eight-link budget")
	}

	actual := vec{}
	for _, link := range result.links[:result.count] {
		actual = add(actual, link.weight)
	}

	for axis := range 3 {
		if math.Abs(float64(actual[axis]-total[axis])) > 1e-6 {
			t.Fatalf("pruning changed lobe%d energy: got%f want%f", axis, actual[axis], total[axis])
		}
	}
}

func TestTransportPatchesKeepPhysicalSurfaceAndMaterialIdentity(t *testing.T) {
	t.Parallel()

	material := [4]float32{.5, .4, .3, 0}

	first := makeTransportKey(vec{.1, .1, .1}, vec{0, 1, 0}, material)
	if first != makeTransportKey(vec{.7, .7, .7}, normalize(vec{.1, 1, .1}), material) {
		t.Fatal("same coarse physical patch did not coalesce")
	}

	for _, other := range []transportKey{makeTransportKey(vec{1, .1, .1}, vec{0, 1, 0}, material), makeTransportKey(vec{.1, .1, .1}, vec{0, -1, 0}, material), makeTransportKey(vec{.1, .1, .1}, vec{0, 1, 0}, [4]float32{.6, .4, .3, 0}), makeTransportKey(vec{.1, .1, .1}, vec{0, 1, 0}, [4]float32{.5, .4, .3, 1})} {
		if first == other {
			t.Fatal("merged distinct cell, surface orientation, material or sidedness")
		}
	}
}
