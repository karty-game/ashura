//go:build !js

package lightmapbuild

import (
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/xatlas"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func TestProductionUVPageRetainsSubpixelFootprintWithoutCentroidFallback(t *testing.T) {
	t.Parallel()

	source := modeldata.Document{Source: "sliver", Parts: []modeldata.Part{receiverQuad("surface", 0, false)}}
	atlas := xatlas.Atlas{
		Width:  16,
		Height: 16,
		Charts: 1,
		Image:  make([]uint32, 16*16),
		Meshes: []xatlas.Mesh{
			{
				Vertices: []xatlas.Vertex{
					{Original: 0, Chart: 0, UV: [2]float32{.01, .01}},
					{Original: 1, Chart: 0, UV: [2]float32{.03, .01}},
					{Original: 2, Chart: 0, UV: [2]float32{.01, .03}},
				},
				Indices: []uint32{0, 1, 2},
			},
		},
	}
	atlas.Image[0] = 0x80000000

	page, err := productionUVPage(t.Context(), source, xatlas.Page{Atlas: atlas, PartIndices: []int{0}})
	if err != nil {
		t.Fatal(err)
	}

	if page.ChartOwners[0] != 0 || page.GutterOwners[0] != 0 {
		t.Fatal("subpixel footprint missing", page.ChartOwners[0], page.GutterOwners[0])
	}

	for i := 1; i < len(page.ChartOwners); i++ {
		if page.ChartOwners[i] != -1 || page.GutterOwners[i] != math.MaxUint32 {
			t.Fatal("unowned output filled", i)
		}
	}

	model, err := modeldata.Decode(page.Model)
	if err != nil {
		t.Fatal(err)
	}

	if len(model.Parts) != 1 || len(model.Parts[0].Indices) != 12 || len(page.UVs[0]) != 24 || len(page.TriangleCharts[0]) != 1 ||
		page.TriangleCharts[0][0] != 0 {
		t.Fatal("UV-only binding mismatch")
	}

	if len(source.Parts[0].Vertices) != 4*32 || len(source.Parts[0].Indices) != 6*4 {
		t.Fatal("source geometry mutated")
	}
}

// The retained coverage pass must match the former independent receiver pass,
// including source indices skipped by chart coverage and per-part materials.
func TestProductionUVRetainedReceiversMatchIndependentPass(t *testing.T) {
	t.Parallel()

	source := modeldata.Document{
		Source: "binding",
		Parts:  []modeldata.Part{receiverQuad("first", 0, false), receiverQuad("second", 2, false)},
	}
	atlas := xatlas.Atlas{Width: 16, Height: 16, Charts: 2, Image: make([]uint32, 16*16), Meshes: []xatlas.Mesh{
		{Vertices: []xatlas.Vertex{
			{Original: 0, Chart: -1},
			{Original: 0, Chart: -1},
			{Original: 0, Chart: -1},
			{
				Original: 0,
				Chart:    0,
				UV:       [2]float32{2, 2},
			},
			{Original: 1, Chart: 0, UV: [2]float32{6, 2}},
			{Original: 2, Chart: 0, UV: [2]float32{2, 6}},
		}, Indices: []uint32{0, 1, 2, 3, 4, 5}},
		{Vertices: []xatlas.Vertex{
			{
				Original: 0,
				Chart:    1,
				UV:       [2]float32{9, 2},
			},
			{Original: 1, Chart: 1, UV: [2]float32{13, 2}},
			{Original: 2, Chart: 1, UV: [2]float32{9, 6}},
		}, Indices: []uint32{0, 1, 2}},
	}}

	page, err := productionUVPage(t.Context(), source, xatlas.Page{Atlas: atlas, PartIndices: []int{0, 1}})
	if err != nil {
		t.Fatal(err)
	}

	reference, err := bake.ReceiversFromModel(page.Model, page.UVs, page.Width, page.Height)
	if err != nil {
		t.Fatal(err)
	}

	if len(reference) == 0 || !reflect.DeepEqual(page.Receivers, reference) {
		t.Fatal("retained support differs from independently clipped model")
	}

	foundSecond := false

	for _, receiver := range page.Receivers {
		for _, piece := range receiver.Pieces {
			if piece.Triangle == 0 {
				t.Fatal("receiver aliases skipped source triangle")
			}

			if piece.Triangle == 2 {
				foundSecond = true
			}
		}

		for _, patch := range receiver.Patches {
			if patch.Triangle != math.MaxUint32 {
				t.Fatal("receiver is not independent of visibility triangulation")
			}

			if receiver.Pixel%16 >= 9 && patch.Material != 1 {
				t.Fatal("receiver lost material binding")
			}
		}
	}

	if !foundSecond {
		t.Fatal("second material has no covered receiver")
	}
}
