//go:build !js

package assetbuild

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMain(main *testing.M) {
	if os.Getenv("KARTY_MISE_HELPER") == "1" {
		os.Exit(managedMiseHelper())
	}

	if os.Getenv("KARTY_MATERIALIZE_HELPER") == "1" {
		os.Exit(materializeHelper())
	}

	os.Exit(main.Run())
}

func TestMaterializePipelineCacheAndToolChanges(t *testing.T) {
	project, config, options, material := materializeFixture(t)
	if runtime.GOOS == "windows" {
		t.Skip("replacing an executable in place is platform-specific; external tool identity has separate coverage")
	}

	tool := filepath.Join(project, "tool")

	executable, err := os.ReadFile(material.Executable)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The private fixture copy must be executable for its subprocess invocation.
	if err := os.WriteFile(tool, executable, 0o700); err != nil {
		t.Fatal(err)
	}

	material.Executable = tool

	config.Plugins[1] = Materialize(material)
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	assertMaterializeCalls(t, material, 1)

	for _, name := range []string{"brick-albedo.ktx2", "brick-normal.ktx2", "brick-ao.ktx2"} {
		if _, err := os.Stat(filepath.Join(project, "generated", "assets", name)); err != nil {
			t.Fatal(err)
		}
	}

	code, err := fixtureRead(project, "generated/assets/assets.go")
	if err != nil || !bytes.Contains(code, []byte("func BrickNormal()")) || !bytes.Contains(code, []byte("func BrickAO()")) {
		t.Fatalf("missing generated normal/AO declarations: %s %v", code, err)
	}

	warm, err := Build(t.Context(), config, options)
	if err != nil || warm.Executed != 0 {
		t.Fatalf("warm Materialize must not execute: %+v %v", warm, err)
	}

	assertMaterializeCalls(t, material, 1)
	//nolint:gosec // Preserve executable permission while replacing this private test-tool fixture.
	if err := os.WriteFile(tool, append(executable, []byte("changed executable content")...), 0o700); err != nil {
		t.Fatal(err)
	}

	replaced, err := Build(t.Context(), config, options)
	if err != nil || replaced.Executed != 1 {
		t.Fatalf("same-path tool replacement must invalidate only Materialize: %+v %v", replaced, err)
	}

	assertMaterializeCalls(t, material, 2)

	strength := 2.0
	material.NormalStrength = &strength

	config.Plugins[1] = Materialize(material)
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	assertMaterializeCalls(t, material, 3)
	t.Setenv("MATERIALIZE_LOG", "error")

	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	assertMaterializeCalls(t, material, 4)

	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	assertMaterializeCalls(t, material, 4)

	codeAfter, err := fixtureRead(project, "generated/assets/assets.go")
	if err != nil || !bytes.Equal(code, codeAfter) {
		t.Fatal("tool identity/options changes modified identical output catalog")
	}
}

func TestMaterializeFailuresPreservePublishedOutputs(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"fail", "missing", "dimensions", "software", "unknown", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			project, config, options, material := materializeFixture(t)
			if _, err := Build(t.Context(), config, options); err != nil {
				t.Fatal(err)
			}

			before, err := fixtureRead(project, "generated/assets/brick-normal.ktx2")
			if err != nil {
				t.Fatal(err)
			}

			material.Environment["KARTY_MATERIALIZE_MODE"] = mode
			if mode == "timeout" {
				material.Timeout = 50 * time.Millisecond
			}

			config.Plugins[1] = Materialize(material)
			if _, err := Build(t.Context(), config, options); err == nil {
				t.Fatalf("accepted invalid tool mode %q", mode)
			}

			after, err := fixtureRead(project, "generated/assets/brick-normal.ktx2")
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed tool changed previous output")
			}

			material.Environment["KARTY_MATERIALIZE_MODE"] = "ok"
			material.Timeout = defaultMaterializeTimeout

			config.Plugins[1] = Materialize(material)
			if _, err := Build(t.Context(), config, options); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaterializeExplicitSoftwareAndUnidentifiedBackend(t *testing.T) {
	t.Parallel()

	_, config, options, material := materializeFixture(t)
	material.Environment["KARTY_MATERIALIZE_MODE"] = "software"
	material.AllowSoftware = true
	material.BackendIdentity = ""

	config.Plugins[1] = Materialize(material)
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	second, err := Build(t.Context(), config, options)
	if err != nil || second.Executed != 1 {
		t.Fatalf("missing backend identity must disable Materialize cache: %+v %v", second, err)
	}

	assertMaterializeCalls(t, material, 2)
}

func TestMaterializeOptionsAndInputValidation(t *testing.T) {
	t.Parallel()

	project, config, options, material := materializeFixture(t)
	for _, maps := range [][]string{{"height"}, {"normal", "normal"}, {"metallic"}} {
		invalid := material
		invalid.Maps = maps

		config.Plugins[1] = Materialize(invalid)
		if _, err := Build(t.Context(), config, options); err == nil {
			t.Fatalf("accepted unsupported maps %v", maps)
		}
	}

	config.Plugins[1] = Materialize(material)

	fixtureWrite(t, project, "assets/brick-normal.png", materializePNG(t, color.NRGBA{R: 128, G: 128, B: 255, A: 255}))

	if _, err := Build(t.Context(), config, options); err == nil {
		t.Fatal("generated maps silently overwrote authored normal resource")
	}
}

func materializeFixture(t *testing.T) (string, Config, Options, MaterializeOptions) {
	t.Helper()
	project := t.TempDir()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	material := MaterializeOptions{
		Executable: executable, BackendIdentity: "fixture GPU driver@1", Preset: "brick",
		Environment: map[string]string{"KARTY_MATERIALIZE_HELPER": "1", "KARTY_MATERIALIZE_COUNT": filepath.Join(project, "calls")},
	}
	fixtureWrite(t, project, "assets/brick-albedo.png", materializePNG(t, color.NRGBA{R: 100, G: 90, B: 80, A: 255}))

	config := Config{
		Input: []string{"assets/**"}, Output: "generated/assets",
		Plugins: []Plugin{Images(), Materialize(material), Mipmaps(), KTX2(), GoCatalog()},
	}

	return project, config, Options{Project: project, Identity: "Materialize-fixture"}, material
}

func materializePNG(t *testing.T, pixel color.NRGBA) []byte {
	t.Helper()

	value := image.NewNRGBA(image.Rect(0, 0, 4, 4))

	for row := range 4 {
		for column := range 4 {
			value.SetNRGBA(column, row, pixel)
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatal(err)
	}

	return encoded.Bytes()
}

func assertMaterializeCalls(t *testing.T, options MaterializeOptions, expected int) {
	t.Helper()

	data, err := os.ReadFile(options.Environment["KARTY_MATERIALIZE_COUNT"])
	if err != nil || strings.Count(string(data), "call\n") != expected {
		t.Fatalf("tool invocation count wanted %d: %q %v", expected, data, err)
	}
}
