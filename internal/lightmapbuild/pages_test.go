//go:build !js

//nolint:varnamelen // Geometry fixture coordinates use conventional short names.
package lightmapbuild

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/xatlas"
)

var errUnrelatedPageGeometry = errors.New("unrelated malformed geometry")

func TestPreparedPagesUseExternalVisibilityAndOwnReceivers(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	floor := receiverQuad("floor", 0, false)
	// Smooth shading must never tilt the production tracer's physical hemisphere.
	for offset := 0; offset < len(floor.Vertices); offset += 32 {
		putFloat(floor.Vertices[offset+12:], .6)
		putFloat(floor.Vertices[offset+16:], .8)
	}

	ceiling := receiverQuad("external red blocker", 1, true)
	ceiling.Material.Color = [4]float32{1, 0, 0, 1}
	receivers := modeldata.Document{Source: "receiver-chunk", Parts: []modeldata.Part{floor}}
	occluders := modeldata.Document{Source: "whole-space", Parts: []modeldata.Part{floor, ceiling}}

	preparer, err := NewPreparer(ctx, occluders)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = preparer.Close() }()

	results, err := preparer.PreparePages(
		ctx,
		receivers,
		PageOptions{
			RNM: true, PrecomputeTransport: true, PreserveGeometricNormals: true, TransportSamples: 16, Padding: 2,
			TexelsPerUnit: 8,
			MaxPageSize:   32,
			MaxPages:      4,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected one receiver page, got %d", len(results))
	}

	result := results[0]
	if result.Width >= 32 || result.Height >= 32 {
		t.Fatal("small receiver page retained maximum texture allocation")
	}

	model, err := modeldata.Decode(result.Model)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := lightmapdata.Decode(result.Lightmap)
	if err != nil {
		t.Fatal(err)
	}

	if len(model.Parts) != 1 || model.Parts[0].Name != "floor" || len(doc.Triangles)/48 != 4 ||
		doc.ModelSHA256 != sha256.Sum256(result.Model) ||
		!doc.Transport() ||
		doc.GISamples != 16 {
		t.Fatal("page/visibility domain metadata changed")
	}

	covered := 0

	if len(result.GeometricNormals) != result.Width*result.Height*16 {
		t.Fatal("build-only geometric normal buffer lost")
	}

	for offset := 0; offset < len(doc.Receivers); offset += 32 {
		if readFloat(doc.Receivers[offset+12:]) == 0 {
			continue
		}

		covered++

		assertPageGeometricNormal(t, result.GeometricNormals[offset/32*16:], doc.Receivers[offset:])

		if readFloat(doc.Receivers[offset+4:]) != 0 {
			t.Fatal("external blocker incorrectly became a receiver")
		}
	}

	if covered == 0 {
		t.Fatal("no receiver samples")
	}

	scene := newTransportScene(doc.Nodes, preparer.triangles, doc.Surfaces)

	hit, distance := scene.trace(vec{.5, .01, .5}, vec{0, 1, 0})
	if hit < 0 || math.Abs(float64(distance-.99)) > .001 {
		t.Fatalf("external blocker not visible: hit=%d distance=%g", hit, distance)
	}

	if readFloat(doc.Surfaces[hit*16:]) <= 0 || readFloat(doc.Surfaces[hit*16+4:]) != 0 {
		t.Fatal("external material reflectance did not follow its BVH triangle")
	}

	if len(doc.UVs) != 1 || len(doc.UVs[0])*4 != len(model.Parts[0].Vertices) {
		t.Fatal("page UV stream does not match geometry")
	}

	canceled, stop := context.WithCancel(ctx)
	stop()

	if _, err = preparer.PreparePages(canceled, receivers, PageOptions{}); err == nil {
		t.Fatal("accepted canceled preparation")
	}
}

func assertPageGeometricNormal(t *testing.T, geometric, receiver []byte) {
	t.Helper()

	if readFloat(geometric) != 0 || readFloat(geometric[4:]) != 1 || readFloat(geometric[8:]) != 0 ||
		math.Abs(float64(readFloat(receiver[16:])-.6)) > .0001 {
		t.Fatal("geometric normals replaced by interpolated shading normals")
	}
}

func TestPhysicalPageOptionsRejectInvalidValues(t *testing.T) {
	t.Parallel()

	for _, options := range []PageOptions{{TexelsPerUnit: -1}, {TexelsPerUnit: float32(math.NaN())}, {MaxPageSize: 63}, {MaxPageSize: 4096}, {MaxPages: 65}, {Padding: 17}, {PrecomputeTransport: true}} {
		if _, err := normalizePageOptions(options); err == nil {
			t.Fatalf("accepted %#v", options)
		}
	}
}

func receiverQuad(name string, y float32, down bool) modeldata.Part {
	part := modeldata.Part{
		Name:     name,
		Material: modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Roughness: .5, Albedo: -1, Normal: -1, AO: -1},
	}

	normal := float32(1)
	if down {
		normal = -1
	}

	for i, p := range [][3]float32{{0, y, 0}, {1, y, 0}, {1, y, 1}, {0, y, 1}} {
		uv := [4][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
		for _, v := range []float32{p[0], p[1], p[2], 0, normal, 0, uv[i][0], uv[i][1]} {
			part.Vertices = binary.LittleEndian.AppendUint32(part.Vertices, math.Float32bits(v))
		}
	}

	indices := []uint32{0, 2, 1, 0, 3, 2}
	if down {
		indices = []uint32{0, 1, 2, 0, 2, 3}
	}

	for _, i := range indices {
		part.Indices = binary.LittleEndian.AppendUint32(part.Indices, i)
	}

	return part
}

func TestPageTexturePruningPreservesSharedMaterialDependencies(t *testing.T) {
	t.Parallel()

	model := modeldata.Document{
		Textures: []modeldata.Texture{
			{Role: modeldata.Albedo, Data: []byte("unused")},
			{Role: modeldata.Albedo, Data: []byte("color")},
			{Role: modeldata.Normal, Data: []byte("normal")},
		},
		Parts: []modeldata.Part{
			{Material: modeldata.Material{Albedo: 1, Normal: 2, AO: -1}},
			{Material: modeldata.Material{Albedo: 1, Normal: -1, AO: -1}},
		},
	}
	prunePageTextures(&model)

	if len(model.Textures) != 2 || string(model.Textures[0].Data) != "color" || string(model.Textures[1].Data) != "normal" ||
		model.Parts[0].Material.Albedo != 0 ||
		model.Parts[0].Material.Normal != 1 ||
		model.Parts[1].Material.Albedo != 0 ||
		model.Parts[1].Material.Normal != -1 {
		t.Fatal("pruning changed or duplicated live material dependencies")
	}
}

func TestPreparedPagesSpillOnlyOnPortableBudget(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	model := modeldata.Document{Source: "spill fixture"}

	for index := range 16 {
		part := receiverQuad(strconv.Itoa(index), 0, false)
		for offset := 0; offset < len(part.Vertices); offset += 32 {
			putFloat(part.Vertices[offset:], readFloat(part.Vertices[offset:])+float32(index)*2)
		}

		model.Parts = append(model.Parts, part)
	}

	preparer, err := NewPreparer(ctx, model)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = preparer.Close() }()

	options := PageOptions{
		RNM:                 true,
		PrecomputeTransport: true,
		TransportSamples:    16,
		Padding:             4,
		TexelsPerUnit:       8,
		MaxPageSize:         64,
		MaxPages:            64,
	}
	attempts := 0

	results, err := preparer.preparePages(
		ctx,
		model,
		options,
		func(ctx context.Context, model modeldata.Document, page xatlas.Page, options PageOptions) (Result, error) {
			attempts++

			if page.Width == 64 {
				return Result{}, errTransportBudget
			}

			return preparer.preparePage(ctx, model, page, options)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	triangles := 0

	for _, result := range results {
		if result.Width != 32 || result.Height != 32 {
			t.Fatal("budget spill changed the wrong capacity")
		}

		page, err := modeldata.Decode(result.Model)
		if err != nil {
			t.Fatal(err)
		}

		for _, part := range page.Parts {
			triangles += len(part.Indices) / 12
		}
	}

	if triangles != 32 || len(results) < 2 || attempts <= len(results) {
		t.Fatal("budget spill omitted geometry or did not repartition")
	}

	other := errUnrelatedPageGeometry
	attempts = 0

	_, err = preparer.preparePages(
		ctx,
		model,
		options,
		func(context.Context, modeldata.Document, xatlas.Page, PageOptions) (Result, error) {
			attempts++

			return Result{}, other
		},
	)
	if !errors.Is(err, other) || attempts != 1 {
		t.Fatal("retried a non-budget failure")
	}
}

func TestTransportBudgetSentinelPreservesGeometryError(t *testing.T) {
	t.Parallel()
	// Shared fixture backing avoids allocating128MiB just to test accounting.
	// Logical resource lengths must count even if host slices alias each other.
	data := make([]byte, lightmapdata.MaxTransportBytes/8)

	document := &lightmapdata.Document{
		Nodes:           data,
		Triangles:       data,
		Receivers:       data,
		Padding:         data,
		Frames:          data,
		Surfaces:        data,
		ReceiverGIIndex: data,
		GIReceivers:     data,
	}
	if err := transportSize(document); err != nil {
		t.Fatal(err)
	}

	document.GIPatches = []byte{0}
	if err := transportSize(document); !errors.Is(err, errTransportBudget) || !errors.Is(err, errGeometry) {
		t.Fatal("budget classification lost compatibility", err)
	}
}
