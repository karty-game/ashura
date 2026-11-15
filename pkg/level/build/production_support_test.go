//go:build !js

package levelbuild

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func decodeSupportPart(data []byte) (bake.ReconstructionAdjacency, error) {
	if len(data) < supportPartHeader {
		return bake.ReconstructionAdjacency{}, ErrInvalid
	}

	count, targets := binary.LittleEndian.Uint64(data[8:]), binary.LittleEndian.Uint64(data[16:])
	if count > uint64(len(data)/supportRowBytes) || targets > uint64(len(data)/supportEdgeBytes) {
		return bake.ReconstructionAdjacency{}, ErrInvalid
	}
	//nolint:gosec // G115: test allocations are bounded by the admitted input byte count.
	rows := bake.ReconstructionAdjacency{
		Owners:  make([]uint32, 0, int(count)),
		Offsets: make([]uint32, 1, int(count)+1),
		Targets: make([]uint32, 0, int(targets)),
	}

	rows, err := appendSupportPart(rows, data)
	if err != nil {
		return rows, err
	}

	_, err = bake.PrepareReconstructionSupport([]bake.Receiver{{ID: 0xfedcba9876543210}, {ID: 2}}, rows)

	return rows, err
}

func TestProductionSupportRejectsNonGeneratedWeightsAndVisibility(t *testing.T) {
	t.Parallel()

	for _, edge := range []bake.ReconstructionEdge{
		{From: 7, To: 9, Weight: .5, Visible: true},
		{From: 7, To: 9, Weight: 1, Visible: false},
	} {
		if _, err := indexSupportRows(
			bake.SurfaceEdgeReport{Edges: []bake.ReconstructionEdge{edge}},
			[]bake.Receiver{{ID: 7}, {ID: 9}},
		); err == nil {
			t.Fatal("non-generated edge silently converted to unit visible support")
		}
	}
}

func TestProductionSupportPartRoundtripAndAdmission(t *testing.T) {
	t.Parallel()

	edges := bake.ReconstructionAdjacency{
		Owners: []uint32{1, 0}, Offsets: []uint32{0, 1, 2}, Targets: []uint32{0, 1},
	}

	data, err := encodeSupportPart(edges, 0, len(edges.Owners))
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeSupportPart(data)
	if err != nil || !reflect.DeepEqual(decoded, edges) {
		t.Fatal("support graph changed during binary roundtrip", decoded, err)
	}

	for _, corrupt := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"truncated", func(data []byte) []byte { return data[:len(data)-1] }},
		{"magic", func(data []byte) []byte {
			data[0] ^= 1

			return data
		}},
		{"impossible-count", func(data []byte) []byte {
			binary.LittleEndian.PutUint64(data[8:], math.MaxUint64)

			return data
		}},
		{"impossible-target-count", func(data []byte) []byte {
			binary.LittleEndian.PutUint64(data[16:], math.MaxUint64)

			return data
		}},
		{"zero-row-count", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[supportPartHeader+4:], 0)

			return data
		}},
		{"unknown-owner", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[supportPartHeader:], math.MaxUint32)

			return data
		}},
		{"self-edge", func(data []byte) []byte {
			copy(data[supportPartHeader+8:supportPartHeader+12], data[supportPartHeader:supportPartHeader+4])

			return data
		}},
	} {
		t.Run(corrupt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := decodeSupportPart(corrupt.mutate(append([]byte(nil), data...))); err == nil {
				t.Fatal("admitted corrupt surface support")
			}
		})
	}
}

func TestProductionSupportIndexBindsGeometryAndExactOrderedReceivers(t *testing.T) {
	t.Parallel()

	receivers := []bake.Receiver{{ID: 7, Position: bake.Vec3{1, 2, 3}}, {ID: 9, Position: bake.Vec3{4, 5, 6}}}

	receiverHash, err := lightmapper.ReceiverHash(receivers)
	if err != nil {
		t.Fatal(err)
	}

	geometryHash := strings.Repeat("a", 64)

	index := productionSupport{
		Schema:       productionSupportSchema,
		GeometryHash: geometryHash,
		ReceiverHash: receiverHash,
		Count:        2,
		Rows:         2,
		Parts: []assetbuild.Artifact{
			{SHA256: strings.Repeat("b", 64), Size: supportPartHeader + 2*supportRowBytes + 2*supportEdgeBytes},
		},
		Diagnostics: assetbuild.Artifact{SHA256: strings.Repeat("d", 64), Size: supportDiagnosticHeader},
	}
	if err := validateSupportIndex(index, geometryHash, receivers); err != nil {
		t.Fatal("valid bounded graph rejected", err)
	}

	for _, changed := range [][]bake.Receiver{
		{receivers[1], receivers[0]},
		{{ID: 7, Position: bake.Vec3{1, 2, 3}}, {ID: 9, Position: bake.Vec3{4, 5, 7}}},
	} {
		if err := validateSupportIndex(index, geometryHash, changed); err == nil {
			t.Fatal("support reused for different ordered receiver geometry")
		}
	}

	if err := validateSupportIndex(index, strings.Repeat("c", 64), receivers); err == nil {
		t.Fatal("support reused for different visibility geometry")
	}

	for _, mutate := range []func(*productionSupport){
		func(index *productionSupport) { index.Schema = "unknown" },
		func(index *productionSupport) { index.Count = math.MaxUint64 },
		func(index *productionSupport) { index.Count = 1 },
		func(index *productionSupport) { index.Parts = nil },
		func(index *productionSupport) { index.Parts = []assetbuild.Artifact{{Size: supportArtifactBytes + 1}} },
	} {
		changed := index
		mutate(&changed)

		if err := validateSupportIndex(changed, geometryHash, receivers); err == nil {
			t.Fatal("admitted inconsistent graph manifest", changed)
		}
	}
}

func supportDiagnosticFixture() (bake.SurfaceEdgeReport, []bake.Receiver) {
	return bake.SurfaceEdgeReport{
		SegmentCount: 12, CandidateWork: 31, ExpansionWork: 17,
		IsolatedReceiverIDs: []uint64{7, 9},
		AmbiguousContacts: []bake.SurfaceContactDiagnostic{{
			ReceiverIDs: []uint64{7, 9}, Start: bake.Vec3{1, 2, 3}, End: bake.Vec3{4, 5, 6},
			Normals: [2]bake.Vec3{{0, 1, 0}, {0, 0, 1}},
		}},
	}, []bake.Receiver{{ID: 7}, {ID: 9}, {ID: 11}, {ID: 13}}
}

func TestProductionSupportDiagnosticsRoundtripAndPreflight(t *testing.T) {
	t.Parallel()

	report, receivers := supportDiagnosticFixture()

	data, err := encodeSupportDiagnostics(report)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeSupportDiagnostics(data, uint64(len(receivers)))
	if err != nil || !reflect.DeepEqual(decoded, report) {
		t.Fatal("diagnostics changed", decoded, err)
	}

	if err := validateSupportReport(decoded, receivers); err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func([]byte) []byte{
		func(data []byte) []byte { return data[:len(data)-1] },
		func(data []byte) []byte {
			data[0] ^= 1

			return data
		},
		func(data []byte) []byte {
			binary.LittleEndian.PutUint64(data[8:], math.MaxUint64)

			return data
		},
		func(data []byte) []byte {
			binary.LittleEndian.PutUint64(data[16:], math.MaxUint64)

			return data
		},
		func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[supportDiagnosticHeader+16+48:], math.MaxUint32)

			return data
		},
	} {
		if _, err := decodeSupportDiagnostics(mutate(append([]byte(nil), data...)), uint64(len(receivers))); err == nil {
			t.Fatal("corrupt diagnostic payload admitted")
		}
	}

	if _, err := decodeSupportDiagnostics(data, 1); err == nil {
		t.Fatal("receiver bound ignored")
	}
}

func TestProductionSupportReportOwnsEveryIsolationAndRejectsIncidentEdges(t *testing.T) {
	t.Parallel()

	for _, mutate := range []func(*bake.SurfaceEdgeReport){
		func(report *bake.SurfaceEdgeReport) { report.IsolatedReceiverIDs = []uint64{7, 100} },
		func(report *bake.SurfaceEdgeReport) { report.IsolatedReceiverIDs = []uint64{9, 7} },
		func(report *bake.SurfaceEdgeReport) { report.IsolatedReceiverIDs = []uint64{7, 7} },
		func(report *bake.SurfaceEdgeReport) { report.IsolatedReceiverIDs = []uint64{7, 9, 11} },
		func(report *bake.SurfaceEdgeReport) { report.AmbiguousContacts = nil },
		func(report *bake.SurfaceEdgeReport) { report.AmbiguousContacts[0].ReceiverIDs = []uint64{7, 11} },
		func(report *bake.SurfaceEdgeReport) { report.AmbiguousContacts[0].Start[0] = float32(math.NaN()) },
		func(report *bake.SurfaceEdgeReport) {
			report.Edges = []bake.ReconstructionEdge{{From: 7, To: 11, Weight: 1, Visible: true}}
		},
		func(report *bake.SurfaceEdgeReport) {
			report.Edges = []bake.ReconstructionEdge{{From: 11, To: 100, Weight: 1, Visible: true}}
		},
	} {
		report, receivers := supportDiagnosticFixture()
		mutate(&report)

		if err := validateSupportReport(report, receivers); err == nil {
			t.Fatal("invalid isolation report admitted", report)
		}
	}

	report, receivers := supportDiagnosticFixture()

	report.Edges = []bake.ReconstructionEdge{{From: 11, To: 13, Weight: 1, Visible: true}}
	if err := validateSupportReport(report, receivers); err != nil {
		t.Fatal("unaffected support rejected", err)
	}
}

func TestProductionSupportCacheExcludesRadianceAndWorkerTilingSettings(t *testing.T) {
	t.Parallel()

	geometry := &bake.ReconstructionOptions{
		Radius:         .125,
		PlaneTolerance: .015,
		NormalCos:      .95,
		TangentCos:     .9,
		NoiseScale:     2,
		Strength:       .7,
		MaximumHalo:    65536,
	}
	first := canonicalSupportOptions(
		ProductionOptions{
			Reconstruction: geometry,
			SurfaceEdges:   bake.SurfaceEdgeOptions{MaximumCandidates: 1000, MaximumEdges: 500, Ambiguity: bake.SurfaceIsolateAmbiguity},
		},
	)
	other := *geometry
	other.NoiseScale, other.Strength, other.MaximumHalo = 9, .2, 32

	second := canonicalSupportOptions(ProductionOptions{Reconstruction: &other, SurfaceEdges: first.Limits})
	if !reflect.DeepEqual(first, second) {
		t.Fatal("unrelated resolve settings invalidate geometry support", first, second)
	}

	if *geometry == *first.Limits.Support {
		t.Fatal("source options unexpectedly canonicalized in place")
	}

	other.NormalCos = .8

	changed := canonicalSupportOptions(ProductionOptions{Reconstruction: &other, SurfaceEdges: first.Limits})
	if reflect.DeepEqual(first, changed) {
		t.Fatal("geometry eligibility omitted from support identity")
	}

	other.Strength = 0

	disabled := canonicalSupportOptions(ProductionOptions{Reconstruction: &other, SurfaceEdges: first.Limits})
	if disabled.Enabled || disabled.Limits.Support != nil {
		t.Fatal("disabled reconstruction still prepares geometry support", disabled)
	}
}

func TestProductionSupportPageIdentityBindsExactOrderedArtifacts(t *testing.T) {
	t.Parallel()

	geometry := strings.Repeat("a", 64)
	pages := []pageRecord{
		{
			Width:     64,
			Height:    32,
			Receivers: assetbuild.Artifact{SHA256: strings.Repeat("b", 64), Size: 100},
			Geometry:  assetbuild.Artifact{SHA256: strings.Repeat("c", 64), Size: 200},
		},
		{
			Width:     32,
			Height:    16,
			Receivers: assetbuild.Artifact{SHA256: strings.Repeat("d", 64), Size: 300},
			Geometry:  assetbuild.Artifact{SHA256: strings.Repeat("e", 64), Size: 400},
		},
	}

	original, err := productionSupportIdentity(geometry, pages)
	if err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func([]pageRecord){
		func(pages []pageRecord) { pages[0], pages[1] = pages[1], pages[0] },
		func(pages []pageRecord) { pages[0].Receivers.SHA256 = strings.Repeat("f", 64) },
		func(pages []pageRecord) { pages[0].Geometry.SHA256 = strings.Repeat("f", 64) },
		func(pages []pageRecord) { pages[0].Receivers.Size++ },
		func(pages []pageRecord) { pages[0].Width++ },
	} {
		changed := append([]pageRecord(nil), pages...)
		mutate(changed)

		identity, identityErr := productionSupportIdentity(geometry, changed)
		if identityErr != nil || identity == original {
			t.Fatal("different immutable page input reused support", identity, identityErr)
		}
	}

	other, err := productionSupportIdentity(strings.Repeat("f", 64), pages)
	if err != nil || other == original {
		t.Fatal("visibility/material scene identity omitted", other, err)
	}
}

func TestProductionSupportCanonicalOrderingRejectsDuplicatesAcrossParts(t *testing.T) {
	t.Parallel()

	report, receivers := supportDiagnosticFixture()
	for _, edges := range [][]bake.ReconstructionEdge{
		{{From: 11, To: 13, Weight: 1, Visible: true}, {From: 11, To: 13, Weight: 2, Visible: true}},
		{{From: 13, To: 11, Weight: 1, Visible: true}, {From: 11, To: 13, Weight: 1, Visible: true}},
	} {
		report.Edges = edges
		if err := validateSupportReport(report, receivers); err == nil {
			t.Fatal("noncanonical or duplicate graph admitted", edges)
		}
	}

	report.Edges = []bake.ReconstructionEdge{{From: 11, To: 13, Weight: 1, Visible: true}, {From: 13, To: 11, Weight: 1, Visible: true}}
	if err := validateSupportReport(report, receivers); err != nil {
		t.Fatal("canonical graph rejected", err)
	}
}

func validateSupportIndex(index productionSupport, geometryHash string, receivers []bake.Receiver) error {
	receiverHash, err := lightmapper.ReceiverHash(receivers)
	if err != nil {
		return err
	}

	return validateSupportIndexIdentity(index, geometryHash, receiverHash, len(receivers))
}
