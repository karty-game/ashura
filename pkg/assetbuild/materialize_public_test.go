//go:build !js

package assetbuild

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMaterializePublicInstallAndOfflineProbe(t *testing.T) {
	if os.Getenv("KARTY_VERIFY_PUBLIC_TOOLS") != "1" {
		t.Skip("opt-in public release installation and native executable probe")
	}

	if host := os.Getenv("KARTY_VERIFY_TOOL_HOST"); host != "" && host != runtime.GOOS+"-"+runtime.GOARCH {
		t.Fatal("qualification runner does not match its declared native host", host, runtime.GOOS, runtime.GOARCH)
	}

	// Isolate mise's complete state so a warmed developer install cannot satisfy
	// the public distribution check. Mise owns download/extraction verification.
	for _, name := range []string{"MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_CONFIG_DIR", "MISE_STATE_DIR"} {
		t.Setenv(name, t.TempDir())
	}

	t.Setenv("MISE_OFFLINE", "0")
	t.Setenv("MATERIALIZE_CLI", "")
	t.Setenv("CC", filepath.Join(t.TempDir(), "compiler-must-not-run"))
	t.Setenv("CXX", filepath.Join(t.TempDir(), "compiler-must-not-run"))

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()

	executable, err := materializeExecutable(materializePublicContext(t, ctx), MaterializeOptions{})
	if err != nil {
		t.Fatal("public default Materialize installation failed", err)
	}

	notice, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(executable)), "notices", "Materialize-LICENSE"))
	if err != nil || len(notice) == 0 {
		t.Fatal("public installation omitted its Materialize notice", err)
	}

	probePublicMaterialize(t, ctx, executable)

	t.Setenv("MISE_OFFLINE", "1")

	// A new runner has no in-memory snapshot. Mise must reuse its own installed
	// package in offline mode, without an executable override or seeded cache.
	repeated, err := materializeExecutable(materializePublicContext(t, ctx), MaterializeOptions{})
	if err != nil || repeated != executable {
		t.Fatal("public mise installation could not be reused offline", repeated, err)
	}

	probePublicMaterialize(t, ctx, repeated)
}

func materializePublicContext(t *testing.T, ctx context.Context) *Context {
	t.Helper()

	// macOS's system temporary directory may use /var -> /private/var. Select
	// its real owner rather than weakening the pipeline's symlink confinement.
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	run, err := newRunner(Config{Output: "generated"}, Options{Project: project, Identity: "public-materialize-probe"})
	if err != nil {
		t.Fatal(err)
	}

	return &Context{Context: ctx, runner: run}
}

func probePublicMaterialize(t *testing.T, ctx context.Context, executable string) {
	t.Helper()

	for _, argument := range []string{"--version", "--help", "--list-maps"} {
		//nolint:gosec // The module default resolver verifies and fingerprints this pinned release executable.
		command := exec.CommandContext(ctx, executable, argument)
		command.WaitDelay = materializeWaitDelay
		output := &materializeLog{}
		command.Stdout, command.Stderr = output, output

		if err := command.Run(); err != nil {
			t.Fatal("public executable probe failed", argument, err, output.String())
		}

		text := output.String()
		if text == "" || argument == "--version" && !strings.Contains(text, materializeToolVersion) ||
			argument == "--list-maps" && !strings.Contains(strings.ToLower(text), "normal") {
			t.Fatal("public executable probe returned unexpected output", argument, text)
		}
	}
}
