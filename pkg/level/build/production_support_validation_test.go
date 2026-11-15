//go:build !js

package levelbuild

import (
	"errors"
	"math"
	"reflect"
	"testing"

	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func TestProductionSupportDirectPartAdmission(t *testing.T) {
	t.Parallel()

	receivers := []bake.Receiver{{ID: 7}, {ID: 0}, {ID: math.MaxUint64}}
	rows := bake.ReconstructionAdjacency{Owners: []uint32{1, 0}, Offsets: []uint32{0, 2, 3}, Targets: []uint32{0, 2, 1}}

	decoded := bake.ReconstructionAdjacency{Owners: make([]uint32, 0, 2), Offsets: make([]uint32, 1, 3), Targets: make([]uint32, 0, 3)}
	for row := range rows.Owners {
		data, err := encodeSupportPart(rows, row, row+1)
		if err != nil {
			t.Fatal(err)
		}

		decoded, err = appendSupportPart(decoded, data)
		if err != nil {
			t.Fatal(err)
		}
	}

	if !reflect.DeepEqual(decoded, rows) {
		t.Fatal("direct row decode changed dense identities")
	}

	if _, err := bake.PrepareReconstructionSupport(receivers, decoded); err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func(*bake.ReconstructionAdjacency){
		func(value *bake.ReconstructionAdjacency) { value.Owners[1] = value.Owners[0] },
		func(value *bake.ReconstructionAdjacency) { value.Targets[1] = value.Targets[0] },
		func(value *bake.ReconstructionAdjacency) { value.Targets[1] = math.MaxUint32 },
		func(value *bake.ReconstructionAdjacency) { value.Isolated = []uint32{0} },
		func(value *bake.ReconstructionAdjacency) { value.Offsets[1] = 0 },
	} {
		candidate := bake.ReconstructionAdjacency{
			Owners:  append([]uint32(nil), rows.Owners...),
			Offsets: append([]uint32(nil), rows.Offsets...),
			Targets: append([]uint32(nil), rows.Targets...),
		}
		mutate(&candidate)

		if _, err := bake.PrepareReconstructionSupport(receivers, candidate); err == nil {
			t.Fatal("invalid canonical, bounded or isolated support admitted")
		}
	}
}

func TestProductionSupportCanonicalGroupsRetainIdentityBoundaries(t *testing.T) {
	t.Parallel()

	receivers := []bake.Receiver{{ID: 0}, {ID: 7}, {ID: math.MaxUint64}}
	report := bake.SurfaceEdgeReport{Edges: []bake.ReconstructionEdge{
		{From: 0, To: 7, Weight: 1, Visible: true},
		{From: 0, To: math.MaxUint64, Weight: 1, Visible: false},
		{From: 7, To: 0, Weight: 1, Visible: true},
		{From: math.MaxUint64, To: 0, Weight: 1, Visible: true},
	}}

	if err := validateSupportReport(report, receivers); err != nil {
		t.Fatal("valid canonical source groups rejected", err)
	}

	if err := validateSupportReport(bake.SurfaceEdgeReport{}, nil); err != nil {
		t.Fatal("empty report rejected", err)
	}
}

func TestProductionSupportCanonicalGroupsKeepCompleteAdmission(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*bake.SurfaceEdgeReport, *[]bake.Receiver)
	}{
		{"unknown-group-source", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[0].From = 10 }},
		{"unknown-later-destination", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[1].To = 19 }},
		{"isolated-group-source", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[0].From = 7 }},
		{"isolated-later-destination", func(report *bake.SurfaceEdgeReport, receivers *[]bake.Receiver) {
			*receivers = append(*receivers, bake.Receiver{ID: 1})

			report.Edges[0].To, report.Edges[1].To = 1, 7
		}},
		{"later-edge-nan", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[1].Weight = float32(math.NaN()) }},
		{"later-edge-self", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[1].To = report.Edges[1].From }},
		{"duplicate-edge", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[1] = report.Edges[0] }},
		{"noncanonical-group", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.Edges[2].From = report.Edges[0].From }},
		{"duplicate-owner", func(_ *bake.SurfaceEdgeReport, receivers *[]bake.Receiver) {
			*receivers = append(*receivers, (*receivers)[0])
		}},
		{"missing-contact", func(report *bake.SurfaceEdgeReport, _ *[]bake.Receiver) { report.AmbiguousContacts = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			report, receivers := supportDiagnosticFixture()
			receivers = append(receivers, bake.Receiver{ID: 17})

			report.Edges = []bake.ReconstructionEdge{
				{From: 11, To: 13, Weight: 1, Visible: true},
				{From: 11, To: 17, Weight: 1, Visible: true},
				{From: 13, To: 11, Weight: 1, Visible: true},
				{From: 17, To: 11, Weight: 1, Visible: true},
			}
			if err := validateSupportReport(report, receivers); err != nil {
				t.Fatal("invalid control fixture", err)
			}

			if _, err := indexSupportRows(report, receivers); err != nil {
				t.Fatal("valid generated graph rejected during dense admission", err)
			}

			test.mutate(&report, &receivers)

			if err := validateSupportReport(report, receivers); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid canonical support admitted", err)
			}

			if _, err := indexSupportRows(report, receivers); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid generated graph admitted during dense conversion", err)
			}
		})
	}
}
