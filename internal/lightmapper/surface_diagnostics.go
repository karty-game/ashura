package lightmapper

import "slices"

type SurfaceAmbiguityPolicy string

const (
	SurfaceRejectAmbiguity  SurfaceAmbiguityPolicy = "reject"
	SurfaceIsolateAmbiguity SurfaceAmbiguityPolicy = "isolate"
)

type SurfaceContactDiagnostic struct {
	ReceiverIDs []uint64 `json:"receiverIds"`
	Start       Vec3     `json:"start"`
	End         Vec3     `json:"end"`
	Normals     [2]Vec3  `json:"normals"`
}

// SurfaceEdgeReport retains evidence when an explicitly selected conservative
// policy isolates receivers with unproven reconstruction topology.
type SurfaceEdgeReport struct {
	Edges               []ReconstructionEdge       `json:"edges"`
	IsolatedReceiverIDs []uint64                   `json:"isolatedReceiverIds"`
	AmbiguousContacts   []SurfaceContactDiagnostic `json:"ambiguousContacts"`
	SegmentCount        uint64                     `json:"segmentCount"`
	CandidateWork       uint64                     `json:"candidateWork"`
	ExpansionWork       uint64                     `json:"expansionWork"`
}

func (grid *surfaceGrid) isolate(first, second surfaceSegment, firstConflicts, secondConflicts []uint32) {
	owners := map[uint32]bool{first.owner: true, second.owner: true}
	for _, owner := range firstConflicts {
		owners[owner] = true
	}

	for _, owner := range secondConflicts {
		owners[owner] = true
	}

	record := SurfaceContactDiagnostic{Start: first.a, End: first.b, Normals: [2]Vec3{first.normal, second.normal}}

	for owner := range owners {
		grid.isolated[owner] = true
		record.ReceiverIDs = append(record.ReceiverIDs, grid.receivers[owner].ID)
	}

	slices.Sort(record.ReceiverIDs)
	grid.diagnostics = append(grid.diagnostics, record)
}

func (grid *surfaceGrid) pruneIsolated() {
	for owner, links := range grid.adjacency {
		if grid.isolated[uint32(owner)] {
			grid.adjacency[owner] = nil

			continue
		}

		kept := links[:0]
		for _, neighbor := range links {
			if !grid.isolated[neighbor] {
				kept = append(kept, neighbor)
			}
		}

		grid.adjacency[owner] = kept
	}
}
