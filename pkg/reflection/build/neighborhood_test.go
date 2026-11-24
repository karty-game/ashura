//go:build !js

package reflectionbuild

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/reflection/scene"
)

func TestNeighborhoodCacheKeepsDistantChunkEditsOutOfProbeBake(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	copyRoomSources(t, project)

	config := assetbuild.Config{Output: "generated/assets", Pipelines: []assetbuild.Pipeline{{
		Name: "reflections", Import: []assetbuild.Plugin{Scene(fixtureOptions())},
		Process: []assetbuild.Plugin{assetbuild.ReflectionProbes()},
		Output:  []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
	}}}
	options := assetbuild.Options{Project: project, Identity: "neighborhood-cache-test"}
	run := func(status string) assetbuild.Report {
		t.Helper()

		report, err := assetbuild.Build(t.Context(), config, options)
		if err != nil {
			t.Fatal(err)
		}

		found := false

		for _, action := range report.Actions {
			if action.Processor == "reflection-panorama" {
				found = true

				if action.Status != status {
					t.Fatalf("probe bake: got %s, want %s", action.Status, status)
				}
			}
		}

		if !found {
			t.Fatal("missing probe bake action")
		}

		return report
	}
	run("MISS")

	texture := filepath.Join(project, config.Output, "room-reflection.ktx2")
	before := readFixture(t, texture)

	stamp, err := os.Stat(texture)
	if err != nil {
		t.Fatal(err)
	}

	if report := run("HIT"); report.Executed != 0 || report.Written != 0 {
		t.Fatal("warm build computed or rewrote output", report)
	}

	far := chunk{Space: "room", Cell: [3]int{4, 0, 0}, Objects: []reflectionscene.Object{fixtureBox("distant box", 128)}}
	writeChunk(t, project, "distant.json", far)

	if report := run("HIT"); report.Executed != 1 {
		t.Fatal("distant chunk should update only runtime scene", report)
	}

	code := readFixture(t, filepath.Join(project, config.Output, "room.go"))
	if !bytes.Contains(code, []byte("distant box")) || !bytes.Equal(before, readFixture(t, texture)) {
		t.Fatal("runtime scene and reused reflection output diverged")
	}

	after, err := os.Stat(texture)
	if err != nil || !after.ModTime().Equal(stamp.ModTime()) {
		t.Fatal("distant edit rewrote the probe texture", err)
	}

	far.Objects[0].Surface.Color.R = .1
	writeChunk(t, project, "distant.json", far)
	run("HIT")

	near := chunk{Space: "room", Cell: [3]int{1, 0, 0}, Objects: []reflectionscene.Object{fixtureBox("neighbor box", 20)}}
	writeChunk(t, project, "neighbor.json", near)
	run("MISS")

	near.Objects[0].Surface.Color.G = .1
	writeChunk(t, project, "neighbor.json", near)
	run("MISS")

	if err := os.Remove(filepath.Join(project, "assets", "chunks", "neighbor.json")); err != nil {
		t.Fatal(err)
	}

	// Removing the neighbor restores the original local inputs, already cached.
	run("HIT")

	// A shared sky edit invalidates the same local bake even with no geometry edit.
	path := filepath.Join(project, "assets", "room.json")

	var source settings
	if err := json.Unmarshal(readFixture(t, path), &source); err != nil {
		t.Fatal(err)
	}

	source.Fill.SkyColor.R = .2
	writeJSON(t, path, source)
	run("MISS")

	source.ProbePosition.X = 1
	writeJSON(t, path, source)
	run("MISS")

	current := readFixture(t, texture)
	if err := os.WriteFile(texture, []byte("damaged output"), 0o600); err != nil {
		t.Fatal(err)
	}

	if report := run("HIT"); report.Executed != 0 || !bytes.Equal(current, readFixture(t, texture)) {
		t.Fatal("corrupt published image was not restored from the verified cache", report)
	}
}

func TestSourceRejectsGeometryOutsideDeclaredChunk(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	copyRoomSources(t, project)

	part := chunk{Space: "room", Objects: []reflectionscene.Object{fixtureBox("crossing boundary", 16)}}
	writeChunk(t, project, "room.json", part)

	if _, _, err := loadSource(&assetbuild.Plan{Project: project}, fixtureOptions()); err == nil {
		t.Fatal("crossing geometry could escape neighborhood dependencies")
	}
}

func TestProbeNeighborhoodIncludesBoundaryNeighborAndKeepsSpacesSeparate(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	copyRoomSources(t, project)

	left := chunk{Space: "room", Objects: []reflectionscene.Object{fixtureBox("owner", 0)}}
	right := chunk{Space: "room", Cell: [3]int{1, 0, 0}, Objects: []reflectionscene.Object{fixtureBox("neighbor", 17)}}
	other := chunk{Space: "other", Objects: []reflectionscene.Object{fixtureBox("unrelated overlap", 0)}}

	writeChunk(t, project, "room.json", left)
	writeChunk(t, project, "neighbor.json", right)
	writeChunk(t, project, "overlap.json", other)

	path := filepath.Join(project, "assets", "room.json")

	var source settings
	if err := json.Unmarshal(readFixture(t, path), &source); err != nil {
		t.Fatal(err)
	}

	source.ProbePosition = reflectionscene.Vector3{X: 15.5, Y: 3}
	writeJSON(t, path, source)

	_, local, err := loadSource(&assetbuild.Plan{Project: project}, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}

	if len(local.Objects) != 2 ||
		slices.ContainsFunc(local.Objects, func(object reflectionscene.Object) bool { return object.Name == "unrelated overlap" }) {
		t.Fatal("chunk neighborhood joined unrelated space", local.Objects)
	}

	objects := primitives(local.Objects)

	index, _ := nearest(objects, vectorOf(local.ProbePosition), vector{1, 0, 0})
	if index < 0 || objects[index].object.Name != "neighbor" {
		t.Fatal("probe at a chunk boundary did not capture its neighbor")
	}
}

func TestReflectionDistanceAndOpaqueSkyFallback(t *testing.T) {
	t.Parallel()

	authored := authoredRoom(t)
	authored.ProbePosition = reflectionscene.Vector3{Y: 3}
	authored.Objects = []reflectionscene.Object{fixtureBox("outside capture", 34)}

	if index, _ := nearest(primitives(authored.Objects), vectorOf(authored.ProbePosition), vector{1, 0, 0}); index >= 0 {
		t.Fatal("geometry beyond 32 m contributed to the probe")
	}

	image, err := render(t.Context(), authored)
	if err != nil {
		t.Fatal(err)
	}

	for offset := 3; offset < len(image.Pix); offset += 4 {
		if image.Pix[offset] != 255 {
			t.Fatal("out-of-range geometry left a transparent probe pixel")
		}
	}
}

func fixtureBox(name string, x float32) reflectionscene.Object {
	return reflectionscene.Object{
		Name:     name,
		Shape:    reflectionscene.Box,
		Size:     reflectionscene.Vector3{X: 1, Y: 1, Z: 1},
		Position: reflectionscene.Vector3{X: x, Y: 3},
		Surface:  reflectionscene.Surface{Color: reflectionscene.Color{R: .5, G: .5, B: .5, A: 1}},
	}
}

func writeChunk(t *testing.T, project, name string, value chunk) {
	t.Helper()
	writeJSON(t, filepath.Join(project, "assets", "chunks", name), value)
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()

	// #nosec G304 -- Callers select files in the isolated temporary test project.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
