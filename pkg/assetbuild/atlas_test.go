//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"math"
	"slices"
	"strings"
	"testing"
)

func TestAtlasDeterminismRolesAndGutters(t *testing.T) {
	t.Parallel()
	options := atlasTestOptions(t, AtlasOptions{
		Name: "ui", Include: []string{"ui/**"}, MaxSize: 32, Padding: 2, MipLevels: 2, Clamp: true,
	})
	red, blue := [4]byte{255, 0, 0, 255}, [4]byte{0, 0, 255, 255}
	images := []Image{
		atlasTestImage("Red", UsageAlbedo, 3, 5, red),
		atlasTestImage("Blue", UsageAlbedo, 5, 3, blue),
		atlasTestImage("Normal", UsageNormal, 3, 3, [4]byte{128, 128, 255, 255}),
	}

	first, err := packAtlas(images, options)
	if err != nil {
		t.Fatal(err)
	}

	slices.Reverse(images)

	second, err := packAtlas(images, options)
	if err != nil {
		t.Fatal(err)
	}

	encodedFirst, err := EncodeImageSet(first)
	if err != nil {
		t.Fatal(err)
	}

	encodedSecond, err := EncodeImageSet(second)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encodedFirst, encodedSecond) {
		t.Fatal("source traversal order changed packed output")
	}

	if len(first.Images) != 2 || len(first.Regions) != 3 {
		t.Fatalf("role groups: %d pages, %d regions", len(first.Images), len(first.Regions))
	}

	colors := map[string][4]byte{"Red": red, "Blue": blue, "Normal": {128, 128, 255, 255}}

	for _, region := range first.Regions {
		page := atlasTestFindPage(t, first, region.Page)
		if !page.Atlas || page.MaxMipLevels != 2 || len(page.Levels) != 1 {
			t.Fatalf("unsafe page mip policy: %+v", page)
		}

		for y := region.Y - options.Padding; y < region.Y+region.Height+options.Padding; y++ {
			for x := region.X - options.Padding; x < region.X+region.Width+options.Padding; x++ {
				offset := (y*page.Width + x) * atlasChannels
				if got := [4]byte(page.Levels[0][offset : offset+atlasChannels]); got != colors[region.Name] {
					t.Fatalf("region %s gutter (%d,%d) = %v", region.Name, x, y, got)
				}
			}
		}
	}
}

func TestAtlasPackingPageBoundsAndOverflow(t *testing.T) {
	t.Parallel()
	options := atlasTestOptions(t, AtlasOptions{Name: "ui", Include: []string{"**"}, MaxSize: 8, Clamp: true})
	images := []Image{
		atlasTestImage("First", UsageAlbedo, 6, 6, [4]byte{255, 0, 0, 255}),
		atlasTestImage("Second", UsageAlbedo, 6, 6, [4]byte{0, 255, 0, 255}),
	}

	set, err := packAtlas(images, options)
	if err != nil {
		t.Fatal(err)
	}

	if len(set.Images) != 2 || set.Images[0].Name == set.Images[1].Name {
		t.Fatalf("expected independently named pages, got %+v", set.Images)
	}

	for _, region := range set.Regions {
		if region.X != 1 || region.Y != 1 || region.PageWidth != 8 || region.PageHeight != 8 {
			t.Fatalf("wrong region bounds: %+v", region)
		}
	}

	images[0].Width = 7

	images[0].Levels = [][]byte{make([]byte, 7*6*atlasChannels)}
	if _, err := packAtlas(images, options); err == nil {
		t.Fatal("accepted image that fits without gutters but exceeds page with them")
	}
}

func TestAtlasRejectsUnsafeInputsAndOptions(t *testing.T) {
	t.Parallel()

	valid := AtlasOptions{Name: "ui", Include: []string{"**"}, MaxSize: 32, Clamp: true}

	for name, modify := range map[string]func(*AtlasOptions){
		"repeat":             func(options *AtlasOptions) { options.Clamp = false },
		"implicit-selection": func(options *AtlasOptions) { options.Include = nil },
		"non-power-of-two":   func(options *AtlasOptions) { options.MaxSize = 30 },
		"oversized":          func(options *AtlasOptions) { options.MaxSize = 8192 },
		"negative-padding":   func(options *AtlasOptions) { options.Padding = -1 },
		"unsafe-mip-gutters": func(options *AtlasOptions) { options.MipLevels, options.Padding = 3, 2 },
		"negative-mips":      func(options *AtlasOptions) { options.MipLevels = -1 },
		"escaping-name":      func(options *AtlasOptions) { options.Name = "../escape" },
		"empty-pattern":      func(options *AtlasOptions) { options.Include = []string{""} },
		"invalid-pattern":    func(options *AtlasOptions) { options.Include = []string{"["} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := valid
			modify(&options)

			if _, err := normalizedAtlasOptions(options); err == nil {
				t.Fatal("accepted unsafe atlas options")
			}
		})
	}

	options := atlasTestOptions(t, valid)
	for name, modify := range map[string]func(*Image){
		"packed":           func(image *Image) { image.Atlas = true },
		"mipped":           func(image *Image) { image.Levels = append(image.Levels, make([]byte, atlasChannels)) },
		"unknown-role":     func(image *Image) { image.Usage = "roughness" },
		"malformed-pixels": func(image *Image) { image.Levels = [][]byte{{1}} },
		"empty-levels":     func(image *Image) { image.Levels = nil },
		"negative-width":   func(image *Image) { image.Width = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			image := atlasTestImage("Image", UsageAlbedo, 2, 2, [4]byte{1, 2, 3, 255})
			modify(&image)

			if _, err := packAtlas([]Image{image}, options); err == nil {
				t.Fatal("accepted incompatible atlas input")
			}
		})
	}

	duplicate := atlasTestImage("Duplicate", UsageAlbedo, 1, 1, [4]byte{1, 2, 3, 255})
	if _, err := packAtlas([]Image{duplicate, duplicate}, options); err == nil {
		t.Fatal("accepted duplicate region names")
	}
}

func TestAtlasSelectionAndPlanning(t *testing.T) {
	t.Parallel()

	plan := &Plan{Resources: []Resource{
		{ID: "ui/button", Ref: Ref{Action: "button", Slot: "data"}},
		{ID: "ui/menu/icon", Ref: Ref{Action: "icon", Slot: "data"}},
		{ID: "brick-albedo", Ref: Ref{Action: "brick", Slot: "data"}},
	}}
	for _, resource := range plan.Resources {
		atlasTestProducer(t, plan, resource.Ref.Action, ImageSetSchema)
	}

	if err := Atlas(AtlasOptions{Name: "ui", Include: []string{"ui/**"}, Clamp: true}).Plan(plan); err != nil {
		t.Fatal(err)
	}

	if len(plan.Resources) != 2 || plan.Resources[0].ID != "atlas/ui" || plan.Resources[1].ID != "brick-albedo" {
		t.Fatalf("unselected resources were lost: %+v", plan.Resources)
	}

	if plan.Resources[1].Ref != (Ref{Action: "brick", Slot: "data"}) {
		t.Fatalf("unselected resource reference changed: %+v", plan.Resources[1])
	}

	if len(plan.actions) != 4 || len(plan.actions[3].Inputs) != 2 {
		t.Fatalf("expected one group action with two dependencies: %+v", plan.actions)
	}

	if _, _, err := selectAtlasResources(plan.Resources, []string{"["}); err == nil {
		t.Fatal("accepted invalid include pattern")
	}
}

func TestAtlasRejectsPreparedResourceSchema(t *testing.T) {
	t.Parallel()

	plan := &Plan{Resources: []Resource{{ID: "ui/button", Ref: Ref{Action: "prepared", Slot: imageDataSlot}}}}
	atlasTestProducer(t, plan, "prepared", PreparedSetSchema)

	err := Atlas(AtlasOptions{Name: "ui", Include: []string{"ui/**"}, Clamp: true}).Plan(plan)
	if err == nil || !strings.Contains(err.Error(), ImageSetSchema) {
		t.Fatalf("expected image-schema requirement before execution, got %v", err)
	}
}

func TestAtlasSafeMipAlignment(t *testing.T) {
	t.Parallel()
	options := atlasTestOptions(t, AtlasOptions{Name: "ui", Include: []string{"**"}, MaxSize: 32, MipLevels: 3, Clamp: true})
	images := []Image{
		atlasTestImage("Red", UsageAlbedo, 3, 5, [4]byte{255, 0, 0, 255}),
		atlasTestImage("Blue", UsageAlbedo, 5, 3, [4]byte{0, 0, 255, 255}),
	}

	pages, err := layoutAtlas(images, options)
	if err != nil {
		t.Fatal(err)
	}

	alignment := 1 << (options.MipLevels - 1)

	for _, page := range pages {
		for _, placement := range page.placements {
			if placement.x%alignment != 0 || placement.y%alignment != 0 ||
				placement.width%alignment != 0 || placement.height%alignment != 0 {
				t.Fatalf("placement crosses deepest mip grid: %+v", placement)
			}
		}
	}
}

func TestAtlasMipFilteringDoesNotCrossRegions(t *testing.T) {
	t.Parallel()
	options := atlasTestOptions(t, AtlasOptions{Name: "ui", Include: []string{"**"}, MaxSize: 32, MipLevels: 3, Clamp: true})
	red, blue := [4]byte{255, 0, 0, 255}, [4]byte{0, 0, 255, 255}

	set, err := packAtlas([]Image{
		atlasTestImage("Red", UsageAlbedo, 3, 5, red),
		atlasTestImage("Blue", UsageAlbedo, 5, 3, blue),
	}, options)
	if err != nil {
		t.Fatal(err)
	}

	colors := map[string][4]byte{"Red": red, "Blue": blue}

	for _, region := range set.Regions {
		page, mipErr := GenerateMipmaps(atlasTestFindPage(t, set, region.Page))
		if mipErr != nil {
			t.Fatal(mipErr)
		}

		if len(page.Levels) != options.MipLevels {
			t.Fatalf("generated %d mip levels, want %d", len(page.Levels), options.MipLevels)
		}

		for level, pixels := range page.Levels {
			scale := float64(int(1) << level)
			minX := int(math.Floor(float64(region.X)/scale - 0.5))
			maxX := int(math.Ceil(float64(region.X+region.Width)/scale - 0.5))
			minY := int(math.Floor(float64(region.Y)/scale - 0.5))
			maxY := int(math.Ceil(float64(region.Y+region.Height)/scale - 0.5))
			width := max(page.Width>>level, 1)

			for y := minY; y <= maxY; y++ {
				for x := minX; x <= maxX; x++ {
					offset := (y*width + x) * atlasChannels
					if got := [4]byte(pixels[offset : offset+atlasChannels]); got != colors[region.Name] {
						t.Fatalf("%s mip %d bilinear footprint (%d,%d) = %v", region.Name, level, x, y, got)
					}
				}
			}
		}
	}
}

func TestAtlasGeneratedPageNameCollisionsRejected(t *testing.T) {
	t.Parallel()

	var metadata PreparedSet

	for _, name := range []string{"ui", "Ui"} {
		options := atlasTestOptions(t, AtlasOptions{Name: name, Include: []string{"**"}, MaxSize: 8, Clamp: true})

		packed, err := packAtlas([]Image{atlasTestImage("Button", UsageAlbedo, 2, 2, [4]byte{1, 2, 3, 255})}, options)
		if err != nil {
			t.Fatal(err)
		}

		for _, page := range packed.Images {
			metadata.Images = append(metadata.Images, PreparedImage{
				Name: page.Name, Source: page.Source, Usage: page.Usage, Output: page.Source + ".ktx2",
			})
		}
	}

	if metadata.Images[0].Name != metadata.Images[1].Name {
		t.Fatal("fixture no longer exercises case-normalized symbol collision")
	}

	if _, err := catalogCode(metadata); err == nil {
		t.Fatal("accepted ambiguous generated page symbols")
	}
}

func TestAtlasDottedNameThroughKTX2AndCatalog(t *testing.T) {
	t.Parallel()

	images := []Image{
		atlasTestImage("First", UsageAlbedo, 6, 6, [4]byte{255, 0, 0, 255}),
		atlasTestImage("Second", UsageAlbedo, 6, 6, [4]byte{0, 255, 0, 255}),
		atlasTestImage("Normal", UsageNormal, 2, 2, [4]byte{128, 128, 255, 255}),
		atlasTestImage("AO", UsageAO, 2, 2, [4]byte{255, 255, 255, 255}),
		atlasTestImage("SSBump", UsageSSBump, 2, 2, [4]byte{85, 85, 85, 255}),
	}
	config := Config{Output: "generated/assets", Plugins: []Plugin{
		atlasTestSourcePlugin(images),
		Atlas(AtlasOptions{Name: "ui.v1", Include: []string{"**"}, MaxSize: 8, Clamp: true}),
		Mipmaps(), KTX2(), GoCatalog(),
	}}

	project := t.TempDir()
	if _, err := Build(t.Context(), config, Options{Project: project, Identity: "atlas-dotted-name"}); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"atlas/ui.v1-albedo-page-0.ktx2", "atlas/ui.v1-albedo-page-1.ktx2",
		"atlas/ui.v1-normal-page-0.ktx2", "atlas/ui.v1-ao-page-0.ktx2", "atlas/ui.v1-ssbump-page-0.ktx2",
	}

	code, err := fixtureRead(project, "generated/assets/assets.go")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "assets.go", code, parser.AllErrors); err != nil {
		t.Fatal(err)
	}

	for _, name := range expected {
		data, readErr := fixtureRead(project, "generated/assets/"+name)
		if readErr != nil || len(data) == 0 {
			t.Fatalf("missing page %s: %v", name, readErr)
		}

		if !bytes.Contains(code, []byte("//go:embed \""+name+"\"")) {
			t.Fatalf("catalog does not embed complete dotted page identity %s", name)
		}
	}

	data, err := fixtureRead(project, "generated/assets/atlas.json")
	if err != nil {
		t.Fatal(err)
	}

	var regions []AtlasRegion
	if err := json.Unmarshal(data, &regions); err != nil {
		t.Fatal(err)
	}

	pages := make(map[string]bool)
	for _, region := range regions {
		if !token.IsIdentifier(region.Page) || !strings.HasPrefix(region.Page, "UiV1") || pages[region.Page] {
			t.Fatalf("invalid or colliding generated page identity: %+v", region)
		}

		pages[region.Page] = true
	}

	if len(pages) != len(expected) {
		t.Fatalf("got %d unique pages, expected %d", len(pages), len(expected))
	}
}

func atlasTestOptions(t *testing.T, options AtlasOptions) AtlasOptions {
	t.Helper()

	normalized, err := normalizedAtlasOptions(options)
	if err != nil {
		t.Fatal(err)
	}

	return normalized
}

func atlasTestImage(name, usage string, width, height int, pixel [4]byte) Image {
	data := make([]byte, width*height*atlasChannels)
	for offset := 0; offset < len(data); offset += atlasChannels {
		copy(data[offset:offset+atlasChannels], pixel[:])
	}

	return Image{Name: name, Source: name + ".png", Usage: usage, Width: width, Height: height, Levels: [][]byte{data}}
}

func atlasTestFindPage(t *testing.T, set ImageSet, name string) Image {
	t.Helper()

	for _, image := range set.Images {
		if image.Name == name {
			return image
		}
	}

	t.Fatalf("page %q missing among %d prepared pages", name, len(set.Images))

	return Image{}
}

func atlasTestProducer(t *testing.T, plan *Plan, name, schema string) {
	t.Helper()

	_, err := plan.Add(Action{
		ID: name, Processor: "fixture", Revision: "1",
		Outputs: []Slot{{Name: imageDataSlot, Schema: schema}},
		Run:     func(*Context) (Result, error) { return Result{imageDataSlot: nil}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
}

func atlasTestSourcePlugin(images []Image) Plugin {
	return fixturePlugin(func(plan *Plan) error {
		for _, image := range images {
			refs, err := plan.Add(Action{
				ID: image.Name, Processor: "fixture", Revision: "1", Options: image.Name,
				Outputs: []Slot{{Name: imageDataSlot, Schema: ImageSetSchema}},
				Run:     func(*Context) (Result, error) { return imageResult(ImageSet{Images: []Image{image}}) },
			})
			if err != nil {
				return err
			}

			plan.Resources = append(plan.Resources, Resource{ID: image.Name, Ref: refs[0]})
		}

		return nil
	})
}
