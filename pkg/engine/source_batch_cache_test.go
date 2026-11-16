package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/render"
)

//nolint:paralleltest // AllocsPerRun must remain isolated on Go 1.27 and TinyGo.
func TestSourceBatchCacheWarmFramesDoNotAllocate(t *testing.T) {
	sources := make([]int, 17)

	for _, count := range []int{34, 512} {
		world := NewWorld()

		geometry := fixtureTriangle()
		for index := range count {
			world.Add(DefaultTransform(), instanceFixture{geometryFixture: geometry, source: &sources[index%len(sources)]})
		}

		var (
			frame render.Frame
			err   error
		)
		for range 3 {
			frame, err = makeFrame(world, 640, 480)
			if err != nil {
				t.Fatal(err)
			}
		}

		if len(frame.Draws) != len(sources) {
			t.Fatal("alternating sources did not create independent batches", len(frame.Draws))
		}

		allocations := testing.AllocsPerRun(100, func() { frame, err = makeFrame(world, 640, 480) })
		if allocations != 0 || err != nil {
			t.Fatalf("warmed alternating source batches: entities=%d allocations=%g err=%v", count, allocations, err)
		}

		instances := 0
		for _, draw := range frame.Draws {
			instances += len(draw.Instances)
		}

		if instances != count {
			t.Fatal("group cache omitted instances", instances)
		}

		t.Logf("entities=%d alternating sources=%d fixed frame allocations=%g", count, len(sources), allocations)
	}
}

func TestSourceBatchCacheRetiresReferencesAndReadmitsInactiveGroups(t *testing.T) {
	t.Parallel()

	storage := frameStorage{}
	storage.batch(1)

	first, second := &entityBindings{}, &entityBindings{}
	source := new(int)
	draw := render.DrawCall{ID: 1, InstanceKey: source, Color: [4]float32{1}}
	storage.append(draw, first)
	draw.ID, draw.Color = 2, [4]float32{0, 1}
	storage.append(draw, second)
	storage.finish()

	group := first.sourceBatch
	if group != second.sourceBatch || len(group.others) != 1 || !group.active {
		t.Fatal("shared source did not retain distinct material batches")
	}

	// A binding can outlive a frame in which its dynamic mesh identity is absent.
	// Its inactive cache must neither retain the old asset nor bypass admission.
	group.firstKey.Impostor = &impostordata.Document{}

	storage.reset()
	storage.finish()

	if group.active || group.key != (sourceKey{}) || group.firstKey != (materialKey{}) || group.first != nil || group.others != nil ||
		len(storage.groups) != 0 {
		t.Fatal("retired group retained source/material/draw references")
	}

	storage.reset()

	draw.ID = 1
	storage.append(draw, first)

	frame := storage.finish()
	if first.sourceBatch == group || !first.sourceBatch.active || len(frame.Draws) != 1 || len(frame.Draws[0].Instances) != 1 ||
		storage.groups[sourceKey{mesh: source}] != first.sourceBatch {
		t.Fatal("returning binding reused a retired group outside the registry")
	}
}

func TestSourceBatchCacheTracksIdentityProbeAndUnbatchedTransitions(t *testing.T) {
	t.Parallel()

	storage := frameStorage{}
	storage.batch(1)

	entry := &entityBindings{}
	sources := [2]int{}
	keys := []sourceKey{{mesh: &sources[0]}, {mesh: &sources[1]}, {mesh: &sources[1], probe: 7}, {mesh: &sources[0]}}

	var previous *sourceBatches

	for _, key := range keys {
		storage.reset()
		storage.append(render.DrawCall{ID: 1, InstanceKey: key.mesh, ProbeID: key.probe}, entry)

		frame := storage.finish()
		if entry.sourceBatch == previous || !entry.sourceBatch.active || entry.sourceBatch.key != key || len(storage.groups) != 1 ||
			len(frame.Draws) != 1 || frame.Draws[0].InstanceKey != key.mesh || frame.Draws[0].ProbeID != key.probe {
			t.Fatal("dynamic source/probe change retained the previous grouping")
		}

		if previous != nil && (previous.active || previous.key != (sourceKey{})) {
			t.Fatal("changed source/probe retained an unused active group")
		}

		previous = entry.sourceBatch
	}

	storage.reset()
	storage.append(render.DrawCall{ID: 1}, entry)
	storage.finish()

	if entry.sourceBatch != nil || previous.active || len(storage.groups) != 0 {
		t.Fatal("unbatched identity retained the previous group")
	}
}

func TestSourceBatchCacheRefreshesWithComponentStructure(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	source := new(int)
	entity := world.Add(DefaultTransform(), instanceFixture{geometryFixture: fixtureTriangle(), source: source})
	geometryFrame(t, world)

	group := world.bindings[entity].sourceBatch
	if group == nil {
		t.Fatal("renderable binding did not retain its admitted source group")
	}

	if err := world.Set(entity, unrelatedDrawFixture{value: 1}); err != nil {
		t.Fatal(err)
	}

	if world.bindings[entity].sourceBatch != nil {
		t.Fatal("structural binding refresh retained stale grouping admission")
	}

	geometryFrame(t, world)

	if world.bindings[entity].sourceBatch != group {
		t.Fatal("unchanged source could not readmit the existing registered group")
	}

	if !world.Remove(entity) {
		t.Fatal("fixture entity removal failed")
	}

	geometryFrame(t, world)

	if group.active || group.key != (sourceKey{}) || len(world.frame.groups) != 0 {
		t.Fatal("last entity removal retained its source group")
	}
}
