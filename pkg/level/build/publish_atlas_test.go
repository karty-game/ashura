//go:build !js

package levelbuild

import (
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestAtlasPlanningSharesOnlyExactPageConversions(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	options := PublishOptions{MaxAtlasSize: 2048, sharedAtlases: &atlasActionCache{}}
	file := codecFixtureFile()
	page := publishedPage{
		Page:  leveldata.Page{Width: 512, Height: 512, Scale: 1, Encoding: "rgbe8", Representation: leveldata.RNMFit},
		Files: []fileRecord{file},
	}
	artifacts := []assetbuild.Artifact{file.Artifact}

	first, err := addConsolidation(&plan, "BC", "chunk", []publishedPage{page}, artifacts, options)
	if err != nil {
		t.Fatal(err)
	}

	repeated, err := addConsolidation(&plan, "Mobile", "chunk", []publishedPage{page}, artifacts, options)
	if err != nil || first[0] != repeated[0] {
		t.Fatal("identical atlas was repeated across profiles", err)
	}

	for index, change := range []func(*publishedPage, *PublishOptions){
		func(page *publishedPage, _ *PublishOptions) { page.Page.Scale = 2 },
		func(page *publishedPage, _ *PublishOptions) { page.Page.Representation = leveldata.RGBDiffuse },
		func(page *publishedPage, _ *PublishOptions) { page.Page.Preview = true },
		func(page *publishedPage, _ *PublishOptions) { page.Page.Maps[0].SHA256 = "different-map" },
		func(page *publishedPage, _ *PublishOptions) { page.Page.UV.Path = "different-uv" },
		func(page *publishedPage, _ *PublishOptions) {
			page.Page.Materials = []leveldata.Material{{Name: "different-material"}}
		},
		func(_ *publishedPage, options *PublishOptions) { options.MaxAtlasSize = 1024 },
		func(_ *publishedPage, options *PublishOptions) { options.Development = true },
	} {
		changed, settings := page, options
		change(&changed, &settings)

		refs, addErr := addConsolidation(&plan, "profile", "chunk", []publishedPage{changed}, artifacts, settings)
		if addErr != nil || refs[0] == first[0] {
			t.Fatal("different atlas conversion aliased", index, addErr)
		}
	}

	changedArtifact := []assetbuild.Artifact{{SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 4096}}

	refs, err := addConsolidation(&plan, "changed-artifact", "chunk", []publishedPage{page}, changedArtifact, options)
	if err != nil || refs[0] == first[0] {
		t.Fatal("different artifact admission aliased", err)
	}
}
