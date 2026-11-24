//go:build !js

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestRepositoryGalleryOrderAndTitles(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	packages, err := listWebPackages(t.Context(), root, "./samples/...")
	if err != nil {
		t.Fatal(err)
	}

	want := []gallerySample{
		{Name: "Basic scene", Path: "apps/samples/basic-scene/"},
		{Name: "Materials", Path: "apps/samples/standard-materials/"},
		{Name: "UI Components", Path: "apps/samples/ui/"},
		{Name: "Sprites2D", Path: "apps/samples/sprites2d/"},
		{Name: "Particles", Path: "apps/samples/particles/"},
		{Name: "Billboards", Path: "apps/samples/billboards/"},
		{Name: "Decals", Path: "apps/samples/decals/"},
		{Name: "Impostors", Path: "apps/samples/impostors/"},
		{Name: "Instancing", Path: "apps/samples/instancing/"},
		{Name: "Reflections", Path: "apps/samples/reflections/"},
		{Name: "Compute texture", Path: "apps/samples/compute-texture/"},
		{Name: "Luminous", Path: "apps/samples/luminous/"},
		{Name: "Sponza", Path: "apps/samples/sponza/"},
		{Name: "Collision courtyard", Path: "apps/samples/courtyard/"},
	}
	if len(packages) != len(want) {
		t.Fatalf("discovered %d samples, want %d", len(packages), len(want))
	}

	samples := make([]gallerySample, len(packages))
	for index, item := range packages {
		samples[index] = gallerySample{
			Name:        item.Name,
			Path:        relativeWebPath(filepath.Join("apps", item.RelativePath)),
			Description: item.Description,
		}
	}

	dist := t.TempDir()
	if err := writeGallery(dist, samples); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The gallery path is fixed within this test's temporary directory.
	page, err := os.ReadFile(filepath.Join(dist, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	links := regexp.MustCompile(`data-sample-path="([^"]+)">([^<]+)</a>`).FindAllSubmatch(page, -1)
	if len(links) != len(want) {
		t.Fatalf("rendered %d gallery links, want %d", len(links), len(want))
	}

	for index, sample := range want {
		if string(links[index][1]) != sample.Path || string(links[index][2]) != sample.Name {
			t.Fatalf("gallery entry %d: got %q at %q, want %q at %q", index, links[index][2], links[index][1], sample.Name, sample.Path)
		}
	}
}
