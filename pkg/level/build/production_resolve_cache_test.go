//go:build !js

package levelbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

type productionResolveCacheFixture struct {
	chunk chunkRecord
	runs  int
}

func (*productionResolveCacheFixture) Name() string { return "production-resolve-cache-fixture" }

func (fixture *productionResolveCacheFixture) Plan(plan *assetbuild.Plan) error {
	plugin := productionPlugin{options: ProductionOptions{BackendIdentity: "fixture-device@1"}}
	action := plugin.chunkResolveAction("fixture", fixture.chunk, assetbuild.Ref{}, nil, assetbuild.Ref{})
	// Preserve the real resolve cache identity while replacing GPU execution
	// with publication of the metadata which resolve carries through unchanged.
	action.Inputs, action.Artifacts = nil, nil
	action.Run = func(_ *assetbuild.Context) (assetbuild.Result, error) {
		fixture.runs++

		data, err := encode(fixture.chunk.Pages)
		if err != nil {
			return nil, err
		}

		bundle, err := assetbuild.EncodeBundle(assetbuild.Bundle{"pages.json": data})

		return assetbuild.Result{slot: bundle}, err
	}

	refs, err := plan.Add(action)
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func TestProductionResolveCacheTracksAllPreservedPageMetadata(t *testing.T) {
	t.Parallel()

	pages := productionSamplePagesFixture()
	pages[0].UVLayout, pages[1].UVLayout = pages[0].Receivers, pages[1].Receivers
	pages[0].SSBump = map[string]assetbuild.Artifact{"stone": pages[0].Receivers}
	fixture := &productionResolveCacheFixture{chunk: chunkRecord{
		Description: leveldata.Chunk{ID: "space/room"}, Pages: pages,
	}}
	config := assetbuild.Config{
		Output: "generated", Pipelines: []assetbuild.Pipeline{{Name: "fixture", Import: []assetbuild.Plugin{fixture}}},
	}
	options := assetbuild.Options{Project: t.TempDir(), Identity: "resolve-page-metadata@1"}

	for _, step := range []struct {
		name     string
		mutate   func()
		executed int
	}{
		{name: "initial", executed: 1},
		{name: "unchanged", executed: 0},
		{name: "ssbump", executed: 1, mutate: func() { pages[0].SSBump["stone"] = pages[1].Receivers }},
		{name: "uv-layout", executed: 1, mutate: func() { pages[1].UVLayout = pages[0].Geometry }},
		{name: "page-order", executed: 1, mutate: func() { pages[0], pages[1] = pages[1], pages[0] }},
	} {
		if step.mutate != nil {
			step.mutate()
		}

		previousRuns := fixture.runs

		report, err := assetbuild.Build(t.Context(), config, options)
		if err != nil || report.Executed != step.executed || fixture.runs != previousRuns+step.executed ||
			report.Reused != 1-step.executed {
			t.Fatal("resolve metadata edit did not invalidate the real action", step.name, report, fixture.runs, err)
		}

		data, err := os.ReadFile(filepath.Join(options.Project, "generated", "pages.json"))
		if err != nil {
			t.Fatal(err)
		}

		expected, err := encode(pages)
		if err != nil || !bytes.Equal(data, expected) {
			t.Fatal("resolve cache returned stale preserved metadata", step.name, string(data), string(expected), err)
		}
	}
}
