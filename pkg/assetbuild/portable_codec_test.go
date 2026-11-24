//go:build !js

package assetbuild

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/pkg/texture"
)

type portableCodecFixture struct{ test *testing.T }

func (portableCodecFixture) Name() string { return "portable-codecs" }
func (fixture portableCodecFixture) Plan(plan *Plan) error {
	refs, err := plan.Add(
		Action{
			ID:        "codec",
			Processor: "portable-codec-fixture",
			Revision:  "1",
			Outputs:   []Slot{{Name: "files", Schema: "karty.files@1"}},
			Run: func(ctx *Context) (Result, error) {
				files := fixture.run(ctx)
				encoded, err := EncodeBundle(files)

				return Result{"files": encoded}, err
			},
		},
	)
	if err == nil {
		plan.Publish(refs[0])
	}

	return err
}
func (fixture portableCodecFixture) run(ctx context.Context) Bundle {
	t := fixture.test
	t.Helper()

	levels := make([][]byte, 0, 4)
	for i, extent := range []int{8, 4, 2, 1} {
		pixels := make([]byte, extent*extent*4)
		for offset := 0; offset < len(pixels); offset += 4 {
			copy(pixels[offset:], []byte{[]byte{35, 65, 95, 125}[i], 80, 140, 255})
		}

		levels = append(levels, pixels)
	}

	source, err := EncodeTexture(Image{Name: "colors", Source: "colors.png", Usage: UsageAlbedo, Width: 8, Height: 8, Levels: levels})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareOpaqueLDRUASTC(ctx, source, BlockCompressionOptions{})
	if err != nil {
		t.Fatal("default WASI UASTC", err)
	}

	files := Bundle{}

	for _, target := range []texture.Format{texture.BC1, texture.BC7, texture.ETC2RGB} {
		encoded, err := TranscodeLDRNative(ctx, prepared, target, BlockCompressionOptions{})
		if err != nil {
			t.Fatal(target, err)
		}

		image, err := ktx2data.Decode(encoded)
		if err != nil || image.Format != target || image.ColorSpace != texture.SRGB || len(image.Levels) != 4 {
			t.Fatal("native mip/transfer admission", target, err)
		}

		files[strconv.Itoa(int(target))+".ktx2"] = encoded
	}

	etc1s, err := PrepareETC1S(ctx, source, 192)
	if err != nil {
		t.Fatal("default WASI ETC1S", err)
	}

	if _, err = basistranscode.Inspect(etc1s); err != nil {
		t.Fatal("ETC1S profile admission", err)
	}

	if _, err = TranscodeAlbedoNative(ctx, etc1s, texture.ETC2RGB, BlockCompressionOptions{}); err != nil {
		t.Fatal("ETC1S native export", err)
	}

	linear, err := EncodeTexture(
		Image{Name: "coefficients", Source: "coefficients.png", Usage: UsageLightmap, Width: 8, Height: 8, Levels: levels},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = CompressSSBumpASTC(ctx, linear, BlockCompressionOptions{}); err != nil {
		t.Fatal("numeric ASTC", err)
	}

	if _, err = CompressSSBumpETC2(ctx, linear, BlockCompressionOptions{}); err != nil {
		t.Fatal("numeric ETC2", err)
	}

	for offset := 3; offset < len(levels[0]); offset += 4 {
		levels[0][offset] = 128
	}

	rgbe, err := EncodeTexture(Image{Name: "hdr", Source: "hdr.png", Usage: UsageLightmap, Width: 8, Height: 8, Levels: levels[:1]})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = CompressRGBERNM(ctx, rgbe, BlockCompressionOptions{}); err != nil {
		t.Fatal("default WASI BC6H", err)
	}

	mobile, err := CompressRGBEETC2(ctx, rgbe, BlockCompressionOptions{})
	if err != nil {
		t.Fatal("RGBE ETC2", err)
	}

	image, err := ktx2data.Decode(mobile)
	if err != nil {
		t.Fatal(err)
	}

	for at := 0; at < len(image.Levels[0]); at += 16 {
		if image.Levels[0][at] != 128 {
			t.Fatal("RGBE exponent changed")
		}
	}

	return files
}

func TestDefaultPortableCodecsNeedNoExecutablesAndReuseCache(t *testing.T) {
	// No shell, Node, mise or native encoder can be selected by this build.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CC", "must-not-run")
	t.Setenv("CXX", "must-not-run")
	config := Config{Output: "prepared", Pipelines: []Pipeline{{Name: "portable", Output: []Plugin{portableCodecFixture{test: t}}}}}

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	options := Options{Project: project, Identity: "portable-codec-qualified-1"}
	if report, err := Build(t.Context(), config, options); err != nil || report.Executed != 1 {
		t.Fatal("cold portable build", report, err)
	}

	if report, err := Build(t.Context(), config, options); err != nil || report.Executed != 0 || report.Reused != 1 {
		t.Fatal("warm portable cache", report, err)
	}
}
