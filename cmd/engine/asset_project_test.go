//go:build !js

package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/texture/ktx2"
)

func TestAssetPreviewOptions(t *testing.T) {
	t.Parallel()

	remaining, options, err := parseAssetRunArgs([]string{
		"--profile", "release", "./samples/...", "--addr", "127.0.0.1:0", "--no-cache",
	})
	if err != nil {
		t.Fatal(err)
	}

	if options.profile != "release" || options.target != "js/wasm" || !options.noCache {
		t.Fatalf("asset options: %+v", options)
	}

	if !slices.Equal(remaining, []string{"./samples/...", "--addr", "127.0.0.1:0"}) {
		t.Fatalf("preview arguments: %v", remaining)
	}

	for _, args := range [][]string{{"--profile"}, {"--profile="}, {"--profile", "--addr"}, {"--no-cache=maybe"}} {
		if _, _, err := parseAssetRunArgs(args); err == nil {
			t.Errorf("accepted invalid arguments %v", args)
		}
	}
}

func TestAssetRecipeCLI(t *testing.T) {
	// The recipe must run on the host even when the caller selects WASM flags.
	t.Setenv("GOOS", "js")
	t.Setenv("GOARCH", "wasm")
	t.Setenv("GOFLAGS", "-race -tags=wasm_recipe_must_ignore_target_tags")
	t.Setenv("CGO_ENABLED", "1")

	project := assetRecipeFixture(t)
	// The imported guard has no valid Go files if caller tags leak. If caller
	// cgo leaks, it selects import C and attempts the rejecting compiler path.
	t.Setenv("CC", filepath.Join(project, "must-not-run-cc"))
	t.Setenv("CXX", filepath.Join(project, "must-not-run-cxx"))

	recipe := strings.Replace(recipeFixture,
		`import "github.com/karty-game/ashura/pkg/assetbuild"`,
		`import (_ "example.test/assets/hostguard"; "github.com/karty-game/ashura/pkg/assetbuild")`, 1)
	writeWebFixtures(t, project, map[string]string{
		assetConfigFile:      recipe,
		"hostguard/guard.go": "//go:build !cgo && !js && !wasm_recipe_must_ignore_target_tags\n\npackage hostguard\n",
		"hostguard/cgo.go":   "//go:build cgo\n\npackage hostguard\nimport \"C\"\n",
	})

	var output bytes.Buffer

	options := assetOptions{profile: "dev", target: "js/wasm", log: &output}
	if err := prepareProjectAssets(t.Context(), project, options); err != nil {
		t.Fatal(err)
	}

	assertRecipeExecution(t, &output, false)

	outputs := readRecipeOutputs(t, project)
	assertRecipeTextures(t, outputs)

	before := stampRecipeOutputs(t, project, outputs)
	if err := prepareProjectAssets(t.Context(), project, options); err != nil {
		t.Fatal(err)
	}

	assertRecipeExecution(t, &output, true)
	assertRecipeUnchanged(t, project, outputs, before)

	if err := prepareProjectAssets(
		t.Context(),
		project,
		assetOptions{profile: "dev", target: "js/wasm", noCache: true, log: &output},
	); err != nil {
		t.Fatal(err)
	}

	assertRecipeExecution(t, &output, false)

	assertRecipeUnchanged(t, project, outputs, before)

	if err := prepareProjectAssets(t.Context(), project, assetOptions{profile: "unknown"}); err == nil {
		t.Fatal("custom recipe accepted an unsupported profile")
	}

	assertRecipeUnchanged(t, project, outputs, before)
	// A recipe that fails must not quietly select the default assets folder.
	writeWebFixtures(t, project, map[string]string{assetConfigFile: "invalid Go source"})

	if err := prepareProjectAssets(t.Context(), project, options); err == nil {
		t.Fatal("invalid custom recipe fell back to the default pipeline")
	}

	assertRecipeUnchanged(t, project, outputs, before)
}

func TestAssetRecipePreviewPreparesMissingImports(t *testing.T) {
	// This temporary module has no VCS checkout; production builds keep stamping.
	t.Setenv("GOFLAGS", "-mod=mod -buildvcs=false")
	project := assetRecipeFixture(t)
	writeWebFixtures(t, project, map[string]string{
		"main.go": "package main\nimport \"example.test/assets/generated/assets\"\nfunc main() { _ = assets.UiAlbedoPage0() }\n",
	})

	dist := t.TempDir()
	if err := buildWebPreview(
		t.Context(),
		project,
		dist,
		".",
		webOptions{assets: assetOptions{profile: "dev", target: "js/wasm"}},
	); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{wasmFile, wasmExecFile, "index.html"} {
		info, err := os.Stat(filepath.Join(dist, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("preview output %s: %v", name, err)
		}
	}
}

func assertRecipeExecution(t *testing.T, output *bytes.Buffer, cached bool) {
	t.Helper()

	text := output.String()
	output.Reset()

	start := strings.LastIndex(text, "Processing: ")
	if start < 0 {
		t.Fatalf("recipe did not report processing: %s", text)
	}

	var executed int

	if _, err := fmt.Sscanf(text[start:], "Processing: %d executed", &executed); err != nil {
		t.Fatal(err)
	}

	if cached && executed != 0 || !cached && executed < 1 {
		t.Fatalf("cached=%v, actual executions=%d: %s", cached, executed, text)
	}
}

func assetRecipeFixture(t *testing.T) string {
	t.Helper()
	project := t.TempDir()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	// Isolated consumers retain the module's exact dependency graph and sums.
	// Recipe execution deliberately does not mutate their module manifests.
	sourceModule, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}

	sums, err := os.ReadFile("../../go.sum")
	if err != nil {
		t.Fatal(err)
	}

	module := strings.Replace(string(sourceModule), "module github.com/karty-game/ashura", "module example.test/assets", 1) +
		fmt.Sprintf(
			"\nrequire github.com/karty-game/ashura v0.0.0\nreplace github.com/karty-game/ashura => %s\n",
			strconv.Quote(root),
		)
	writeWebFixtures(t, project, map[string]string{"go.mod": module, "go.sum": string(sums), assetConfigFile: recipeFixture})

	for name, pixel := range map[string]color.NRGBA{
		"red": {R: 255, A: 255}, "blue": {B: 255, A: 255},
	} {
		var encoded bytes.Buffer

		source := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		for index := 0; index < len(source.Pix); index += 4 {
			copy(source.Pix[index:], []byte{pixel.R, pixel.G, pixel.B, pixel.A})
		}

		if err := png.Encode(&encoded, source); err != nil {
			t.Fatal(err)
		}

		writeWebFixtures(t, project, map[string]string{"assets/ui/" + name + ".png": encoded.String()})
	}

	return project
}

const recipeFixture = `//go:build ignore

package main

import "github.com/karty-game/ashura/pkg/assetbuild"

func main() {
    assetbuild.Main(func(env assetbuild.Environment) assetbuild.Config {
        size := 2
        if env.Profile == "release" { size = 4 }
        return assetbuild.Config{
            Input: []string{"assets/**"}, Output: "generated/assets",
            Plugins: []assetbuild.Plugin{
                assetbuild.Images(),
                assetbuild.Resize(assetbuild.ResizeOptions{Width: size, Height: size}),
                assetbuild.Atlas(assetbuild.AtlasOptions{
                    Name: "ui", Include: []string{"ui/**"}, Clamp: true,
                    MaxSize: 16, Padding: 2, MipLevels: 2,
                }),
                assetbuild.Mipmaps(), assetbuild.KTX2(), assetbuild.GoCatalog(),
            },
        }
    })
}
`

func readRecipeOutputs(t *testing.T, project string) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)

	root := filepath.Join(project, "generated", "assets")
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		//nolint:gosec // WalkDir supplies a generated path beneath this test's private project.
		data, err := os.ReadFile(path)
		result[relative] = data

		return err
	}); err != nil {
		t.Fatal(err)
	}

	return result
}

func assertRecipeTextures(t *testing.T, outputs map[string][]byte) {
	t.Helper()

	code := outputs["assets.go"]
	if _, err := parser.ParseFile(token.NewFileSet(), "assets.go", code, parser.AllErrors); err != nil {
		t.Fatal(err)
	}

	textures := 0

	for name, encoded := range outputs {
		if !strings.HasSuffix(name, ".ktx2") {
			continue
		}

		textures++

		data, err := ktx2.Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}

		if data.Width > 16 || data.Height > 16 || len(data.Levels) != 2 {
			t.Fatalf("atlas dimensions/mips: %+v", data)
		}
	}

	if textures != 1 || !bytes.Contains(code, []byte("Region")) {
		t.Fatalf("wanted one atlas page and generated regions; textures=%d, code=%s", textures, code)
	}
}

func stampRecipeOutputs(t *testing.T, project string, outputs map[string][]byte) map[string]time.Time {
	t.Helper()

	before := make(map[string]time.Time)

	for name := range outputs {
		path := filepath.Join(project, "generated", "assets", name)

		stamp := time.Unix(100, 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		before[name] = info.ModTime()
	}

	return before
}

func assertRecipeUnchanged(t *testing.T, project string, expected map[string][]byte, stamps map[string]time.Time) {
	t.Helper()

	actual := readRecipeOutputs(t, project)
	if len(actual) != len(expected) {
		t.Fatalf("output roster changed: %d -> %d", len(expected), len(actual))
	}

	for name, data := range expected {
		if !bytes.Equal(data, actual[name]) {
			t.Fatalf("output %s changed", name)
		}

		info, err := os.Stat(filepath.Join(project, "generated", "assets", name))
		if err != nil {
			t.Fatal(err)
		}

		if !info.ModTime().Equal(stamps[name]) {
			t.Fatalf("unchanged output %s was rewritten", name)
		}
	}
}

//nolint:paralleltest // The recipe inherits a clean native compiler environment.
func TestAssetRecipeCanImportInProcessBakerWithoutNativeTool(t *testing.T) {
	project := assetRecipeFixture(t)
	recipe := strings.Replace(recipeFixture, `import "github.com/karty-game/ashura/pkg/assetbuild"`,
		`import (_ "github.com/karty-game/ashura/pkg/lightmap/bake"; "github.com/karty-game/ashura/pkg/assetbuild")`, 1)
	writeWebFixtures(t, project, map[string]string{assetConfigFile: recipe})

	var output bytes.Buffer
	if err := prepareProjectAssets(t.Context(), project, assetOptions{profile: "dev", target: "js/wasm", log: &output}); err != nil {
		t.Fatal(err)
	}

	assertRecipeExecution(t, &output, false)
}
