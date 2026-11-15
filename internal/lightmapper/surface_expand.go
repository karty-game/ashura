package lightmapper

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
)

type surfaceOwner struct {
	id    uint64
	index uint32
}

// Keep the immutable expansion guides in one cache line per receiver. Footprint
// slices and transport/material fields are not read by surface expansion.
type surfaceSupportReceiver struct {
	id              uint64
	guide           reconstructionGuide
	geometricNormal Vec3
}

func surfaceSupportReceivers(receivers []Receiver) []surfaceSupportReceiver {
	result := make([]surfaceSupportReceiver, len(receivers))
	for index, receiver := range receivers {
		result[index] = surfaceSupportReceiver{
			id: receiver.ID, guide: reconstructionReceiverGuide(receiver), geometricNormal: receiver.GeometricNormal,
		}
	}

	return result
}

type surfaceExpansion struct {
	//nolint:containedctx // One synchronous expansion owns these joined workers; the caller context never escapes preparation.
	ctx         context.Context
	visibility  surfaceVisibility
	receivers   []surfaceSupportReceiver
	neighbors   [][]uint32
	options     SurfaceEdgeOptions
	radius      float32
	work, edges atomic.Uint64
	stopped     atomic.Bool
}

type surfaceVisitSet struct {
	small   []uint32
	stamps  []uint32
	epoch   uint32
	maximum int
}

// Small radius neighborhoods reuse a compact list. Arbitrary larger components
// transparently switch to an epoch array, bounded by the input receiver count;
// no neighbor is dropped and no per-owner hash table is allocated.
func (set *surfaceVisitSet) reset(owner uint32) {
	set.epoch++
	if set.epoch == 0 {
		clear(set.stamps)
		set.epoch = 1
	}

	set.small = set.small[:0]
	if set.stamps != nil {
		set.stamps[owner] = set.epoch
	} else {
		set.small = append(set.small, owner)
	}
}
func (set *surfaceVisitSet) insert(owner uint32) bool {
	if set.stamps != nil {
		if set.stamps[owner] == set.epoch {
			return false
		}

		set.stamps[owner] = set.epoch

		return true
	}

	if slices.Contains(set.small, owner) {
		return false
	}

	const compactCapacity = 64
	if len(set.small) < compactCapacity {
		set.small = append(set.small, owner)

		return true
	}

	set.stamps = make([]uint32, set.maximum)
	for _, previous := range set.small {
		set.stamps[previous] = set.epoch
	}

	set.stamps[owner] = set.epoch

	return true
}

func (state *surfaceExpansion) active() bool {
	if state.stopped.Load() {
		return false
	}

	if state.ctx.Err() != nil {
		state.stopped.Store(true)

		return false
	}

	return true
}
func (state *surfaceExpansion) visit(count uint64) bool {
	if state.work.Add(count) > state.options.MaximumCandidates {
		state.stopped.Store(true)

		return false
	}

	return true
}
func (state *surfaceExpansion) emit(count uint64) bool {
	if state.edges.Add(count) > state.options.MaximumEdges {
		state.stopped.Store(true)

		return false
	}

	return true
}

type surfaceOwnerRay struct {
	ray     Ray
	other   uint32
	visible bool
}

type surfaceOwnerScratch struct {
	seen      surfaceVisitSet
	queue     []uint32
	rays      []surfaceOwnerRay
	triangles []uint32
	edges     []ReconstructionEdge
}

func (state *surfaceExpansion) owner(owner uint32, scratch *surfaceOwnerScratch) []ReconstructionEdge {
	scratch.seen.reset(owner)
	scratch.queue = append(scratch.queue[:0], owner)

	var work uint64

	position := state.receivers[owner].guide.Position
	for first := 0; first < len(scratch.queue); first++ {
		if !state.active() {
			return nil
		}

		for _, next := range state.neighbors[scratch.queue[first]] {
			if !scratch.seen.insert(next) {
				continue
			}

			work++
			if work > state.options.MaximumCandidates {
				state.stopped.Store(true)

				return nil
			}

			delta := Sub(state.receivers[next].guide.Position, position)
			if Dot(delta, delta) > state.radius*state.radius {
				continue
			}

			scratch.queue = append(scratch.queue, next)
		}
	}

	if !state.visit(work) {
		return nil
	}

	state.ownerRays(owner, scratch)

	var count uint64

	for _, ray := range scratch.rays {
		if ray.visible {
			count++
		}
	}

	if !state.emit(count) {
		return nil
	}

	scratch.edges = scratch.edges[:0]
	for _, ray := range scratch.rays {
		if ray.visible {
			scratch.edges = append(scratch.edges, ReconstructionEdge{
				From: state.receivers[owner].id, To: state.receivers[ray.other].id, Weight: 1, Visible: true,
			})
		}
	}

	slices.SortFunc(scratch.edges, func(first, second ReconstructionEdge) int { return cmp.Compare(first.To, second.To) })

	return scratch.edges
}

func (state *surfaceExpansion) ownerRays(owner uint32, scratch *surfaceOwnerScratch) {
	scratch.rays = scratch.rays[:0]
	low, high := Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}

	receiver := state.receivers[owner]
	for _, next := range scratch.queue[1:] {
		if !state.active() {
			return
		}

		other := state.receivers[next]
		if state.options.Support != nil && !reconstructionGuidesEligible(receiver.guide, other.guide, *state.options.Support) {
			continue
		}

		ray := surfaceGuideVisibilityRay(
			receiver.guide.Position,
			other.guide.Position,
			receiver.geometricNormal,
			other.geometricNormal,
			state.options.Tolerance,
		)

		scratch.rays = append(scratch.rays, surfaceOwnerRay{ray: ray, other: next, visible: ray.Max == 0})
		if ray.Max != 0 {
			end := Add(ray.Origin, Scale(ray.Direction, ray.Max))
			low = minVec(low, minVec(ray.Origin, end))
			high = maxVec(high, maxVec(ray.Origin, end))
		}
	}

	var local bool

	scratch.triangles, local = state.visibility.candidates(low, high, scratch.triangles)
	for index := range scratch.rays {
		if !state.active() {
			return
		}

		ray := &scratch.rays[index]
		if ray.visible {
			continue
		}

		if local {
			ray.visible = !state.visibility.occludedCandidates(ray.ray, scratch.triangles)
		} else {
			ray.visible = !state.visibility.occluded(ray.ray)
		}
	}
}

// Independent owner traversals retain their exact directed visibility rays.
// Sorted owner ranges and sorted per-owner destinations produce canonical
// output directly, avoiding a global sort of millions of edge records.
func surfaceExpandEdges(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	adjacency [][]uint32,
	radius float32,
	options SurfaceEdgeOptions,
	work *uint64,
) ([]ReconstructionEdge, error) {
	return surfaceExpandEdgesWithWorkers(ctx, scene, receivers, adjacency, radius, options, work, runtime.GOMAXPROCS(0))
}

func surfaceExpandEdgesWithWorkers(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	adjacency [][]uint32,
	radius float32,
	options SurfaceEdgeOptions,
	work *uint64,
	requestedWorkers int,
) ([]ReconstructionEdge, error) {
	owners := make([]surfaceOwner, len(receivers))
	for index, receiver := range receivers {
		owners[index] = surfaceOwner{id: receiver.ID, index: uint32(index)}
	}

	slices.SortFunc(owners, func(first, second surfaceOwner) int { return cmp.Compare(first.id, second.id) })

	state := surfaceExpansion{
		ctx:        ctx,
		visibility: newSurfaceVisibility(scene),
		receivers:  surfaceSupportReceivers(receivers),
		neighbors:  surfaceNeighbors(adjacency),
		radius:     radius,
		options:    options,
	}
	state.work.Store(*work)

	workers := min(requestedWorkers, len(owners))
	parts := state.expandParallel(owners, workers)

	*work = state.work.Load()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if state.stopped.Load() {
		return nil, fmt.Errorf(
			"surface expansion exceeds configured limits candidates=%d edges=%d: %w",
			options.MaximumCandidates,
			options.MaximumEdges,
			ErrBounds,
		)
	}

	var count int
	for _, part := range parts {
		count += len(part)
	}

	if count == 0 {
		return nil, nil
	}

	result := make([]ReconstructionEdge, 0, count)
	for _, part := range parts {
		result = append(result, part...)
	}

	return result, nil
}

func (state *surfaceExpansion) expandParallel(owners []surfaceOwner, workers int) [][]ReconstructionEdge {
	parts := make([][]ReconstructionEdge, workers)

	var group sync.WaitGroup

	for worker := range workers {
		width, extra := len(owners)/workers, len(owners)%workers
		start := worker*width + min(worker, extra)

		end := start + width
		if worker < extra {
			end++
		}

		group.Go(func() { parts[worker] = state.expandOwners(owners[start:end]) })
	}

	group.Wait()

	return parts
}

func (state *surfaceExpansion) expandOwners(owners []surfaceOwner) []ReconstructionEdge {
	scratch := surfaceOwnerScratch{seen: surfaceVisitSet{maximum: len(state.receivers)}}

	var result []ReconstructionEdge

	for _, owner := range owners {
		if !state.active() {
			return nil
		}

		local := state.owner(owner.index, &scratch)
		if !state.active() {
			return nil
		}

		result = append(result, local...)
	}

	return result
}
