//go:build !js

package lightmapbake

import (
	"context"

	"github.com/karty-game/ashura/internal/lightmapper"
)

type Moments = lightmapper.Moments
type Statistics = lightmapper.Statistics
type QualityPolicy = lightmapper.QualityPolicy
type Allocation = lightmapper.Allocation
type AdaptivePolicy = lightmapper.AdaptivePolicy
type AdaptiveState = lightmapper.AdaptiveState
type ReconstructionSample = lightmapper.ReconstructionSample
type ReconstructionEdge = lightmapper.ReconstructionEdge
type ReconstructionOptions = lightmapper.ReconstructionOptions
type ReconstructionAdjacency = lightmapper.ReconstructionAdjacency
type ReconstructionSupport = lightmapper.ReconstructionSupport

// PrepareReconstructionSupport seals dense adjacency bound to receiver order.
func PrepareReconstructionSupport(receivers []Receiver, adjacency ReconstructionAdjacency) (*ReconstructionSupport, error) {
	return lightmapper.PrepareReconstructionSupport(receivers, adjacency)
}

const CalibrationProfile = lightmapper.CalibrationProfile

var ErrUnconverged = lightmapper.ErrUnconverged

// AllocateProduction freezes a count from an independent pilot. The hardware
// Worker's BakeAdaptive method also executes and checkpoints the complete flow.
func AllocateProduction(pilot Statistics, policy QualityPolicy) (Allocation, error) {
	return lightmapper.AllocateProduction(pilot, policy)
}

// ProjectRNM exposes the calibrated CPU projection reference. Hardware Bake and
// Reconstruct resolve the same constrained fit in compute.
func ProjectRNM(moments Moments) ([3]Vec3, error) { return lightmapper.ProjectRNM(moments) }

type SurfaceEdgeOptions = lightmapper.SurfaceEdgeOptions
type SurfaceAmbiguityPolicy = lightmapper.SurfaceAmbiguityPolicy
type SurfaceEdgeReport = lightmapper.SurfaceEdgeReport
type SurfaceContactDiagnostic = lightmapper.SurfaceContactDiagnostic

const (
	SurfaceRejectAmbiguity  = lightmapper.SurfaceRejectAmbiguity
	SurfaceIsolateAmbiguity = lightmapper.SurfaceIsolateAmbiguity
)

// BuildSurfaceEdgesReport retains diagnostics for explicitly selected isolation.
func BuildSurfaceEdgesReport(
	scene *PreparedScene,
	receivers []Receiver,
	radius float32,
	options SurfaceEdgeOptions,
) (SurfaceEdgeReport, error) {
	return lightmapper.BuildSurfaceEdgesReport(scene, receivers, radius, options)
}

// BuildSurfaceEdges prepares visible reconstruction support from actual world
// patch boundary contact, including UV seams. Numerical proximity alone cannot
// connect surfaces; ambiguous non-manifold contacts fail explicitly.
func BuildSurfaceEdges(
	scene *PreparedScene,
	receivers []Receiver,
	radius float32,
	options SurfaceEdgeOptions,
) ([]ReconstructionEdge, error) {
	return lightmapper.BuildSurfaceEdges(scene, receivers, radius, options)
}

// BuildSurfaceEdgesReportContext prepares supported topology with cooperative
// cancellation. Successful output is identical to BuildSurfaceEdgesReport.
func BuildSurfaceEdgesReportContext(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	radius float32,
	options SurfaceEdgeOptions,
) (SurfaceEdgeReport, error) {
	return lightmapper.BuildSurfaceEdgesReportContext(ctx, scene, receivers, radius, options)
}
