//go:build !js

package build

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestCatalogRejectsAmbiguousNamesAndUnmappedPages(t *testing.T) {
	t.Parallel()

	image := assetbuild.Image{
		Name:   "Icon",
		Source: "icon.png",
		Usage:  assetbuild.UsageAlbedo,
		Width:  2,
		Height: 2,
		Levels: [][]byte{make([]byte, 16)},
	}

	for _, scenario := range []string{"unexported", "noncolor", "unmapped-atlas", "field-collision", "invalid-region", "unsafe-source", "unsafe-region-source", "truncated-pixels", "oversized", "unknown-page", "duplicate-page"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			set := assetbuild.ImageSet{Images: []assetbuild.Image{image}}

			switch scenario {
			case "truncated-pixels":
				set.Images[0].Levels = [][]byte{make([]byte, 15)}
			case "oversized":
				set.Images[0].Width = 4097
			case "duplicate-page":
				set.Images = append(set.Images, image)
			case "unknown-page":
				set.Regions = []assetbuild.AtlasRegion{
					{Name: "Other", Source: "other.png", Page: "Missing", Width: 1, Height: 1, PageWidth: 1, PageHeight: 1},
				}
			case "unexported":
				set.Images[0].Name = "private"
			case "noncolor":
				set.Images[0].Usage = assetbuild.UsageNormal
			case "unsafe-source":
				set.Images[0].Source = "../icon.png"
			case "unmapped-atlas":
				set.Images[0].Atlas, set.Images[0].MaxMipLevels = true, 1
			case "field-collision", "invalid-region", "unsafe-region-source":
				page := image
				page.Name, page.Source, page.Atlas, page.MaxMipLevels = "Page", "page.png", true, 1
				set.Images = append(set.Images, page)
				set.Regions = []assetbuild.AtlasRegion{
					{Name: "Icon", Source: "icon.png", Page: "Page", Width: 2, Height: 2, PageWidth: 2, PageHeight: 2},
				}

				if scenario == "invalid-region" {
					set.Regions[0].Name, set.Regions[0].Width = "Other", 3
				}

				if scenario == "unsafe-region-source" {
					set.Regions[0].Name, set.Regions[0].Source = "Other", "../icon.png"
				}
			default:
				t.Fatal("unknown test scenario")
			}

			if _, err := catalogCode(set); err == nil {
				t.Fatal("invalid catalog was accepted", scenario)
			}
		})
	}
}

func TestEmptyCatalogIsValidGoWithoutRuntimeImports(t *testing.T) {
	t.Parallel()

	code, err := catalogCode(assetbuild.ImageSet{})
	if err != nil {
		t.Fatal(err)
	}

	file, err := parser.ParseFile(token.NewFileSet(), "assets.go", code, parser.AllErrors)
	if err != nil || len(file.Imports) != 0 {
		t.Fatal("empty catalog requires no payload or runtime dependency", string(code), err)
	}
}
