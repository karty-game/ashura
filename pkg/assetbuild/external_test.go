//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func toolFixture(name string, calls *int, observed *string) Config {
	return Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(Action{
			ID: "external", Processor: "external", Revision: "1", Options: name,
			Outputs: []Slot{{Name: "files", Schema: "files@1"}},
			Run: func(ctx *Context) (Result, error) {
				*calls++

				resolved, err := ctx.TrackTool(name)
				if err != nil {
					return nil, err
				}

				*observed = resolved
				data, err := json.Marshal(Bundle{"prepared.bin": []byte("prepared")})

				return Result{"files": data}, err
			},
		})
		if err == nil {
			plan.Publish(refs[0])
		}

		return err
	})}}
}

func TestExternalToolContentAndModeInvalidateWarmCache(t *testing.T) {
	t.Parallel()
	project, tools := t.TempDir(), t.TempDir()
	full := fixtureExecutable(t, tools, "processor.exe", "version one")
	calls, observed := 0, ""
	config := toolFixture(full, &calls, &observed)
	options := Options{Project: project, Identity: "external-content"}
	build := func(executed int) {
		t.Helper()

		report, err := Build(t.Context(), config, options)
		if err != nil || report.Executed != executed {
			t.Fatalf("external build: executed=%d report=%+v err=%v", executed, report, err)
		}
	}

	build(1)
	build(0)

	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, tools, "processor.exe", []byte("version two"))

	if err := os.Chtimes(full, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	build(1)
	build(0)

	if runtime.GOOS != "windows" {
		//nolint:gosec // The isolated tool fixture must remain executable while its permission change invalidates the cache.
		if err := os.Chmod(full, 0o711); err != nil {
			t.Fatal(err)
		}

		build(1)
	}

	if calls < 2 || observed != full {
		t.Fatalf("tool observations: calls=%d resolved=%q expected=%q", calls, observed, full)
	}
}

func TestExternalToolSymlinkRetargetInvalidatesEvenWithIdenticalBytes(t *testing.T) {
	t.Parallel()
	project, tools := t.TempDir(), t.TempDir()
	first := fixtureExecutable(t, tools, "first.exe", "same executable contents")
	second := fixtureExecutable(t, tools, "second.exe", "same executable contents")

	link := filepath.Join(project, "processor.exe")
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("executable symlinks unavailable: %v", err)
	}

	calls, observed := 0, ""

	config, options := toolFixture("./processor.exe", &calls, &observed), Options{Project: project, Identity: "external-symlink"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if calls != 1 || observed != first {
		t.Fatalf("first resolved path %q, calls=%d", observed, calls)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || calls != 2 || observed != second {
		t.Fatalf("retargeted tool: %+v calls=%d resolved=%q err=%v", report, calls, observed, err)
	}
}

func TestExternalToolPathLookupDetectsNewEarlierExecutable(t *testing.T) {
	project, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	secondTool := fixtureExecutable(t, second, "processor.exe", "same executable contents")
	t.Setenv("PATH", strings.Join([]string{first, second}, string(filepath.ListSeparator)))

	calls, observed := 0, ""

	config, options := toolFixture("processor.exe", &calls, &observed), Options{Project: project, Identity: "external-path"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if calls != 1 || observed != secondTool {
		t.Fatalf("initial PATH lookup %q calls=%d", observed, calls)
	}

	firstTool := fixtureExecutable(t, first, "processor.exe", "same executable contents")

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || calls != 2 || observed != firstTool {
		t.Fatalf("earlier PATH entry: %+v calls=%d resolved=%q err=%v", report, calls, observed, err)
	}

	t.Setenv("PATH", second)

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || calls != 3 || observed != secondTool {
		t.Fatalf("changed PATH: %+v calls=%d resolved=%q err=%v", report, calls, observed, err)
	}
}

func TestExternalToolRelativeLookupUsesProjectDirectory(t *testing.T) {
	project := t.TempDir()
	full := fixtureExecutable(t, project, "tools/processor.exe", "tool")
	t.Setenv("PATH", "tools")

	calls, observed := 0, ""

	config, options := toolFixture("processor.exe", &calls, &observed), Options{Project: project, Identity: "relative-tool"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if calls != 1 || observed != full {
		t.Fatalf("relative PATH lookup: calls=%d resolved=%q expected=%q", calls, observed, full)
	}
}

func TestMissingExternalToolNeverReusesSuccessfulResult(t *testing.T) {
	t.Parallel()
	project, tools := t.TempDir(), t.TempDir()
	full := fixtureExecutable(t, tools, "processor.exe", "tool")
	calls, observed := 0, ""

	config, options := toolFixture(full, &calls, &observed), Options{Project: project, Identity: "missing-tool"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}

	report, err := Build(t.Context(), config, options)
	if err == nil || report.Reused != 0 || calls != 2 {
		t.Fatalf("missing executable used cached output: report=%+v calls=%d err=%v", report, calls, err)
	}

	data, err := fixtureRead(project, "generated/assets/prepared.bin")
	if err != nil || string(data) != "prepared" {
		t.Fatalf("failed tool changed published output: %q err=%v", data, err)
	}
}

func TestExternalToolAccessDoesNotRelaxSourceConfinement(t *testing.T) {
	t.Parallel()
	project, tools := t.TempDir(), t.TempDir()
	full := fixtureExecutable(t, tools, "processor.exe", "tool")

	run, err := newRunner(Config{Output: "generated/assets"}, Options{Project: project, Identity: "external-confinement"})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &Context{Context: t.Context(), runner: run}
	if _, err := ctx.ReadFile(full); !errors.Is(err, errPath) {
		t.Fatalf("ordinary source read escaped project: %v", err)
	}

	if _, err := ctx.TrackTool(full); err != nil {
		t.Fatalf("explicit external executable rejected: %v", err)
	}
}

func fixtureExecutable(t *testing.T, root, name, contents string) string {
	t.Helper()
	fixtureWrite(t, root, name, []byte(contents))
	full := filepath.Join(root, filepath.FromSlash(name))
	//nolint:gosec // PATH resolution requires executable permissions on this isolated temporary tool fixture.
	if err := os.Chmod(full, 0o755); err != nil {
		t.Fatal(err)
	}

	return full
}
