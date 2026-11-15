package lightmapper

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
)

func surfaceExpansionFixture(t *testing.T) (*PreparedScene, []Receiver, [][]uint32) {
	t.Helper()
	scene, _ := surfaceEdgeFixture(t)

	const count = 20

	receivers := make([]Receiver, count)

	neighbors := make([][]uint32, count)
	for index := range receivers {
		receivers[index] = Receiver{
			ID:              uint64((index*7)%count + 1),
			Position:        Vec3{float32(index) * .01, .5, 0},
			Normal:          Vec3{0, 0, 1},
			GeometricNormal: Vec3{0, 0, 1},
			Tangent:         Vec3{1, 0, 0},
			Side:            1,
			Handedness:      1,
		}
		if index > 0 {
			neighbors[index] = append(neighbors[index], uint32(index-1))
		}

		if index+1 < count {
			neighbors[index] = append(neighbors[index], uint32(index+1))
		}
	}

	return scene, receivers, neighbors
}

func TestParallelSurfaceExpansionRetainsExactDirectedOrderAndWork(t *testing.T) {
	t.Parallel()
	scene, receivers, neighbors := surfaceExpansionFixture(t)
	options := SurfaceEdgeOptions{MaximumCandidates: 100000, MaximumEdges: 100000}

	var (
		reference    []ReconstructionEdge
		expectedWork uint64
	)

	for _, workers := range []int{1, 2, 7} {
		var work uint64

		edges, err := surfaceExpandEdgesWithWorkers(t.Context(), scene, receivers, neighbors, .031, options, &work, workers)
		if err != nil || len(edges) == 0 {
			t.Fatal(edges, err)
		}

		if workers == 1 {
			reference, expectedWork = edges, work

			continue
		}

		if !reflect.DeepEqual(edges, reference) || work != expectedWork {
			t.Fatal("worker scheduling changed exact support/order/work", workers, work, expectedWork)
		}
	}
}

func TestSurfaceVisitSetPreservesLargeNeighborhoodsAndEpochs(t *testing.T) {
	t.Parallel()

	set := surfaceVisitSet{maximum: 200}
	for range 3 {
		set.reset(199)

		for owner := range uint32(199) {
			if !set.insert(owner) || set.insert(owner) {
				t.Fatal("lost or duplicated large-neighborhood ownership", owner)
			}
		}

		if set.insert(199) {
			t.Fatal("owner revisited")
		}
	}
}

type surfaceCancelContext struct {
	//nolint:containedctx // The fixture forwards one test-owned cancellable context and cancels it at a deterministic polling boundary.
	context.Context

	cancel context.CancelFunc
	calls  atomic.Int32
}

func (ctx *surfaceCancelContext) Err() error {
	if ctx.calls.Add(1) == 3 {
		ctx.cancel()
	}

	return ctx.Context.Err()
}

func TestParallelSurfaceExpansionCancelsAndReturnsDeterministicBounds(t *testing.T) {
	t.Parallel()
	scene, receivers, neighbors := surfaceExpansionFixture(t)
	options := SurfaceEdgeOptions{MaximumCandidates: 100000, MaximumEdges: 1}

	var expected string

	for _, workers := range []int{1, 7} {
		var work uint64
		if _, err := surfaceExpandEdgesWithWorkers(
			t.Context(),
			scene,
			receivers,
			neighbors,
			.031,
			options,
			&work,
			workers,
		); !errors.Is(
			err,
			ErrBounds,
		) {
			t.Fatal("edge limit bypassed", err)
		} else if expected == "" {
			expected = err.Error()
		} else if err.Error() != expected {
			t.Fatal("nondeterministic bound failure", err, expected)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	step := &surfaceCancelContext{Context: ctx, cancel: cancel}
	options.MaximumEdges = 100000

	var work uint64
	if edges, err := surfaceExpandEdgesWithWorkers(
		step,
		scene,
		receivers,
		neighbors,
		.031,
		options,
		&work,
		7,
	); !errors.Is(err, context.Canceled) ||
		edges != nil {
		t.Fatal("canceled expansion published partial support", edges, err)
	}

	if _, err := BuildSurfaceEdgesReportContext(ctx, scene, receivers, 1, options); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled preparation started work", err)
	}
}
