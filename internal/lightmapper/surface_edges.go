package lightmapper

import (
	"context"
	"math"
	"slices"
)

// SurfaceEdgeOptions supplies explicit offline preparation limits. Exceeding a
// bound fails preparation; candidates or weak connections are never truncated.
type SurfaceEdgeOptions struct {
	MaximumCandidates, MaximumEdges uint64
	Tolerance                       float32                // Zero chooses float32 coordinate precision, not a blur radius.
	Support                         *ReconstructionOptions // Optional exact geometry/frame eligibility of the consuming reconstruction pass.
	Ambiguity                       SurfaceAmbiguityPolicy // Empty selects strict rejection.
}

type surfaceSegment struct {
	owner                uint32
	a, b, center, normal Vec3
}
type surfaceContact struct {
	low, high float32
	owner     uint32
}
type surfaceContactSlot struct {
	low, high    float32
	ownerPlusOne uint32 // Zero is empty; admitted receiver indices are below MaxUint32.
}
type surfaceCell [3]int64

// BuildSurfaceEdges derives physical adjacency from positive-length collinear
// footprint boundary contact with opposing interiors. A spatial hash generates
// candidates; proximity alone never establishes connectivity. Radius expansion
// stays in that connected surface component and tests the complete visibility
// scene before admitting each reconstruction link, including cross-chart links.
func BuildSurfaceEdges(
	scene *PreparedScene,
	receivers []Receiver,
	radius float32,
	options SurfaceEdgeOptions,
) ([]ReconstructionEdge, error) {
	if options.Ambiguity == SurfaceIsolateAmbiguity {
		return nil, ErrInvalid
	}

	report, err := BuildSurfaceEdgesReport(scene, receivers, radius, options)

	return report.Edges, err
}

func BuildSurfaceEdgesReport(
	scene *PreparedScene,
	receivers []Receiver,
	radius float32,
	options SurfaceEdgeOptions,
) (SurfaceEdgeReport, error) {
	return BuildSurfaceEdgesReportContext(context.Background(), scene, receivers, radius, options)
}

// BuildSurfaceEdgesReportContext prepares the same complete supported graph with
// cooperative cancellation across boundary, contact and parallel visibility work.
//
//nolint:cyclop // Keep complete option, segment and topology admission before reporting an immutable supported graph.
func BuildSurfaceEdgesReportContext(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	radius float32,
	options SurfaceEdgeOptions,
) (SurfaceEdgeReport, error) {
	if ctx == nil {
		return SurfaceEdgeReport{}, ErrInvalid
	}

	if err := ctx.Err(); err != nil {
		return SurfaceEdgeReport{}, err
	}

	if scene == nil || !receiverFinite(radius) || radius <= 0 || options.MaximumCandidates == 0 || options.MaximumEdges == 0 ||
		!receiverFinite(options.Tolerance) ||
		options.Tolerance < 0 ||
		uint64(len(receivers)) > math.MaxUint32 {
		return SurfaceEdgeReport{}, ErrInvalid
	}

	if options.Ambiguity != "" && options.Ambiguity != SurfaceRejectAmbiguity && options.Ambiguity != SurfaceIsolateAmbiguity {
		return SurfaceEdgeReport{}, ErrInvalid
	}

	if options.Support != nil && (!reconstructionValidOptions(*options.Support) || options.Support.Radius != radius) {
		return SurfaceEdgeReport{}, ErrInvalid
	}

	segments, err := surfaceSegmentsContext(ctx, scene, receivers)
	if err != nil {
		return SurfaceEdgeReport{}, err
	}

	if uint64(len(segments)) > math.MaxUint32 {
		return SurfaceEdgeReport{}, ErrBounds
	}

	grid := newSurfaceGrid(receivers, segments, radius, options)
	if err := grid.insertSegments(ctx); err != nil {
		return SurfaceEdgeReport{}, err
	}

	if err := grid.connectContacts(ctx); err != nil {
		return SurfaceEdgeReport{}, err
	}

	grid.pruneIsolated()
	candidateWork := grid.work

	edges, err := surfaceExpandEdges(ctx, scene, receivers, grid.adjacency, radius, options, &grid.work)
	if err != nil {
		return SurfaceEdgeReport{}, err
	}

	report := SurfaceEdgeReport{
		Edges: edges, AmbiguousContacts: grid.diagnostics,
		SegmentCount: uint64(len(segments)), CandidateWork: candidateWork, ExpansionWork: grid.work - candidateWork,
	}
	for owner := range grid.isolated {
		report.IsolatedReceiverIDs = append(report.IsolatedReceiverIDs, receivers[owner].ID)
	}

	slices.Sort(report.IsolatedReceiverIDs)

	return report, nil
}

func newSurfaceGrid(receivers []Receiver, segments []surfaceSegment, radius float32, options SurfaceEdgeOptions) surfaceGrid {
	return surfaceGrid{
		receivers: receivers,
		segments:  segments,
		options:   options,
		cellSize:  surfaceContactCellSize(radius),
		cells: make(
			map[surfaceCell][]uint32,
		),
		minima:        make([]surfaceCell, len(segments)),
		tolerances:    make([]float32, len(segments)),
		firstContacts: make([]surfaceContactSlot, len(segments)),
		contacts:      make(map[uint32][]surfaceContact),
		adjacency:     make([][]uint32, len(receivers)), isolated: make(map[uint32]bool),
	}
}

//nolint:varnamelen // a/b are canonical endpoint vectors.
func surfaceSegmentKey(a, b Vec3) [6]uint32 {
	less := false

	for i := range 3 {
		if a[i] != b[i] {
			less = a[i] < b[i]

			break
		}
	}

	if !less {
		a, b = b, a
	}

	return [6]uint32{
		math.Float32bits(a[0]),
		math.Float32bits(a[1]),
		math.Float32bits(a[2]),
		math.Float32bits(b[0]),
		math.Float32bits(b[1]),
		math.Float32bits(b[2]),
	}
}

//nolint:mnd,varnamelen // a/b are world endpoints; eight float32 ULPs cover affine boundary interpolation roundoff.
func surfaceTolerance(a, b Vec3, explicit float32) float32 {
	if explicit > 0 {
		return explicit
	}

	tolerance := float32(math.SmallestNonzeroFloat32)

	for _, point := range [2]Vec3{a, b} {
		for _, coordinate := range point {
			value := float32(math.Abs(float64(coordinate)))
			ulp := value - math.Nextafter32(value, 0)
			tolerance = max(tolerance, ulp)
		}
	}

	return tolerance * 8
}

//nolint:varnamelen // a/b are the lower/upper spatial-cell coordinates.
func surfaceCellRange(segment surfaceSegment, radius, tolerance float32, limit uint64) (surfaceCell, surfaceCell, error) {
	var low, high surfaceCell

	cells := float64(1)

	for axis := range 3 {
		a := math.Floor((float64(min(segment.a[axis], segment.b[axis])) - float64(tolerance)) / float64(radius))

		b := math.Floor((float64(max(segment.a[axis], segment.b[axis])) + float64(tolerance)) / float64(radius))
		if a <= math.MinInt64 || b >= math.MaxInt64 {
			return low, high, ErrBounds
		}

		cells *= b - a + 1
		if cells > float64(limit) {
			return low, high, ErrBounds
		}

		low[axis], high[axis] = int64(a), int64(b)
	}

	return low, high, nil
}

//nolint:varnamelen // a/b are the two oriented candidate boundary segments.
func surfaceSegmentsTouch(a, b surfaceSegment, tolerance float32) bool {
	if Dot(a.normal, b.normal) <= 0 {
		return false
	}

	axis := Sub(a.b, a.a)

	length := Length(axis)
	if length <= tolerance {
		return false
	}

	axis = Scale(axis, 1/length)
	if Length(Cross(Sub(b.a, a.a), axis)) > tolerance || Length(Cross(Sub(b.b, a.a), axis)) > tolerance {
		return false
	}

	p, q := Dot(Sub(b.a, a.a), axis), Dot(Sub(b.b, a.a), axis)
	if min(length, max(p, q))-max(float32(0), min(p, q)) <= tolerance {
		return false
	}

	normal := Normalize(Add(a.normal, b.normal))
	if Dot(normal, normal) == 0 {
		return false
	}

	transverse := Normalize(Cross(axis, normal))
	sideA, sideB := Dot(Sub(a.center, a.a), transverse), Dot(Sub(b.center, a.a), transverse)

	return sideA*sideB < -tolerance*tolerance
}

//nolint:varnamelen // a/b are the two candidate boundary segments whose ownership is admitted.
func (grid *surfaceGrid) contactConflicts(index uint32, a, b surfaceSegment, tolerance float32) []uint32 {
	length := Length(Sub(a.b, a.a))
	axis := Scale(Sub(a.b, a.a), 1/length)
	first, second := Dot(Sub(b.a, a.a), axis), Dot(Sub(b.b, a.a), axis)

	low, high := max(float32(0), min(first, second)), min(length, max(first, second))

	previous := grid.firstContacts[index]
	if previous.ownerPlusOne == 0 {
		grid.firstContacts[index] = surfaceContactSlot{low: low, high: high, ownerPlusOne: b.owner + 1}

		return nil
	}

	var conflicts []uint32
	if previous.ownerPlusOne-1 != b.owner && min(high, previous.high)-max(low, previous.low) > tolerance {
		conflicts = append(conflicts, previous.ownerPlusOne-1)
	}

	for _, previous := range grid.contacts[index] {
		if previous.owner != b.owner && min(high, previous.high)-max(low, previous.low) > tolerance {
			conflicts = append(conflicts, previous.owner)
		}
	}

	grid.contacts[index] = append(grid.contacts[index], surfaceContact{low: low, high: high, owner: b.owner})

	return conflicts
}

//nolint:varnamelen // a/b denote one oriented triangle boundary edge.
func surfacePatchSegments(patches []ReceiverPatch, owner uint32) (map[[6]uint32]int, map[[6]uint32]surfaceSegment, error) {
	// Fan diagonals occur twice and are not surface boundaries.
	counts := make(map[[6]uint32]int)
	records := make(map[[6]uint32]surfaceSegment)

	for _, patch := range patches {
		normal := patch.GeometricNormal
		if Dot(normal, normal) == 0 {
			normal = Normalize(Cross(Sub(patch.Positions[1], patch.Positions[0]), Sub(patch.Positions[2], patch.Positions[0])))
		}

		center := Scale(Add(patch.Positions[0], Add(patch.Positions[1], patch.Positions[2])), 1.0/3)
		for i := range 3 {
			a, b := patch.Positions[i], patch.Positions[(i+1)%3]
			if !validVec(a) || !validVec(b) || !validVec(normal) {
				return nil, nil, ErrInvalid
			}

			if a == b {
				continue
			}

			key := surfaceSegmentKey(a, b)
			counts[key]++
			records[key] = surfaceSegment{owner: owner, a: a, b: b, center: center, normal: normal}
		}
	}

	return counts, records, nil
}

func surfaceNeighbors(adjacency [][]uint32) [][]uint32 {
	for _, neighbors := range adjacency {
		slices.Sort(neighbors)
	}

	return adjacency
}

func surfaceSupportEligible(center, neighbor Receiver, options *ReconstructionOptions) bool {
	return options == nil ||
		reconstructionGuidesEligible(reconstructionReceiverGuide(center), reconstructionReceiverGuide(neighbor), *options)
}
