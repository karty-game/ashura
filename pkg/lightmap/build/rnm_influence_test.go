//go:build !js

package lightmapbuild

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
)

func influenceAsset(t *testing.T, document lightmapdata.Document) *lightmap.Asset {
	t.Helper()

	data, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := lightmap.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	return asset
}

func influencePoint(x, y, z, radius float32) lightmap.Point {
	return lightmap.Point{Position: [3]float32{x, y, z}, Color: [3]float32{1, 1, 1}, Intensity: 1, Range: radius}
}

func TestSelectRNMLightsIncludesRemoteReflectors(t *testing.T) {
	t.Parallel()

	document := lightmapfixture.TransportDocument()
	binary.LittleEndian.PutUint32(document.GIPatches, math.Float32bits(100))
	asset := influenceAsset(t, document)
	points := []lightmap.Point{
		influencePoint(0, 2, 0, 4),
		influencePoint(100, 2, 0, 4),
		influencePoint(50, 2, 0, 4),
		influencePoint(1000, 2, 0, 4),
		influencePoint(0, 2, 0, 4),
	}
	points[4].Intensity = 0

	selected, err := SelectRNMLights(asset, points)
	if err != nil || !slices.Equal(selected, points[:2]) {
		t.Fatalf("selection %+v err%v", selected, err)
	}

	selected[0].Intensity = 2
	if points[0].Intensity != 1 {
		t.Fatal("selection aliases authored points")
	}
}

func TestSelectRNMLightsBoundsAndValidation(t *testing.T) {
	t.Parallel()
	asset := influenceAsset(t, lightmapfixture.TransportDocument())

	points := make([]lightmap.Point, MaxRNMInfluenceLights)
	for index := range points {
		points[index] = influencePoint(1000, 0, 0, 1)
	}

	selected, err := SelectRNMLights(asset, points)
	if err != nil || len(selected) != 0 {
		t.Fatalf("distant authored lights rejected: %d %v", len(selected), err)
	}

	oversized := make([]lightmap.Point, MaxRNMInfluenceLights+1)
	copy(oversized, points)

	if _, err := SelectRNMLights(asset, oversized); err == nil {
		t.Fatal("unbounded input admitted")
	}

	for index := range points[:lightmapkernel.RNMMaxPoints+1] {
		points[index] = influencePoint(0, 2, 0, 4)
	}

	if _, err := SelectRNMLights(asset, points); err == nil {
		t.Fatal("exceeded shader light bound")
	}

	for _, point := range []lightmap.Point{
		influencePoint(float32(math.NaN()), 0, 0, 1),
		influencePoint(1000, 0, 0, 0),
		{Position: [3]float32{1000, 0, 0}, Color: [3]float32{2, 0, 0}, Intensity: 1, Range: 1},
	} {
		if _, err := SelectRNMLights(asset, []lightmap.Point{point}); err == nil {
			t.Fatalf("invalid distant point hidden: %+v", point)
		}
	}

	if _, err := SelectRNMLights(nil, nil); err == nil {
		t.Fatal("nil asset admitted")
	}
}

func TestSelectRNMLightsRetainsRayBasedFallback(t *testing.T) {
	t.Parallel()
	asset := influenceAsset(t, lightmapfixture.RNMDocument())
	points := []lightmap.Point{influencePoint(1000, 0, 0, 1)}

	selected, err := SelectRNMLights(asset, points)
	if err != nil || !slices.Equal(selected, points) {
		t.Fatalf("ray fallback lost possible secondary light: %+v %v", selected, err)
	}

	if _, err := SelectRNMLights(asset, make([]lightmap.Point, lightmapkernel.RNMMaxPoints+1)); err == nil {
		t.Fatal("unbounded fallback list admitted")
	}
}

func TestInfluenceBoundsAreConservativeAtSphereEdge(t *testing.T) {
	t.Parallel()

	bounds := influenceBounds{}
	bounds.add([3]float64{-1, 0, 0})
	bounds.add([3]float64{1, 0, 0})

	if !bounds.intersects(influencePoint(4, 4, 0, 5)) || bounds.intersects(influencePoint(4, 4, 0, 4.9)) {
		t.Fatal("sphere/AABB nearest distance incorrect")
	}

	if !bounds.intersects(influencePoint(0, 0, 5, 5)) || bounds.intersects(influencePoint(0, 0, 5, 4.9)) {
		t.Fatal("sphere/AABB depth distance incorrect")
	}
}
