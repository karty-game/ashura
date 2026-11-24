//go:build !js

package levelbuild

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func TestIntegratedProductionReusesExactPreparedSpaceAndSeparatesSources(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	plugin := &productionPlugin{earlyScenes: make(map[string]productionSceneRefs)}
	artifact := codecFixtureFile().Artifact
	page := pageRecord{Geometry: artifact, UVLayout: artifact, Receivers: artifact}
	record := partitioned{Source: "fixture.scene.yaml", Chunks: []chunkRecord{
		{Description: leveldata.Chunk{ID: "first", Space: "room"}, Geometry: artifact, Occluders: artifact, Pages: []pageRecord{page}},
		{Description: leveldata.Chunk{ID: "second", Space: "room"}, Geometry: artifact, Occluders: artifact, Pages: []pageRecord{page}},
	}}
	key := "room/" + artifact.SHA256

	scene, err := plugin.planScene(&plan, "first-source", key, record, record.Chunks[0])
	if err != nil {
		t.Fatal(err)
	}

	plugin.earlyScenes["first-source/"+key] = scene
	// Replanning this scene would collide with its existing action identities.
	// Both chunks must instead depend on the one already prepared space.
	chunks, err := plugin.planProductionSpaces(&plan, "first-source", record)
	if err != nil || len(chunks) != len(record.Chunks) {
		t.Fatal("complete receiver discovery did not reuse its exact prepared scene", chunks, err)
	}

	// Another resource must own distinct scene actions even if its chunks happen
	// to use the same logical-space name and the same occluder bytes.
	if _, err = plugin.planProductionSpaces(&plan, "second-source", record); err != nil {
		t.Fatal("independent source planning collided", err)
	}

	other := scene.geometry

	other.Action = "level/production/geometry/second-source/" + key
	if _, err = plan.SlotSchema(other); err != nil {
		t.Fatal("different source reused the first resource's scene producer", err)
	}
}

func TestIntegratedProductionRejectsInvalidUVBoundsBeforeExecution(t *testing.T) {
	t.Parallel()

	plugin := ProductionBake(ProductionOptions{
		UVs: &UVOptions{MaxPageSize: 1025}, Worker: bake.WorkerOptions{MemoryBytes: 1 << 20}, Preview: true,
	})
	if err := plugin.Plan(&assetbuild.Plan{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("integrated receiver bounds were not admitted during planning", err)
	}
}
