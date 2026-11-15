//go:build !js

package resource

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserCancellationAfterWASMExit(t *testing.T) {
	t.Parallel()
	wasm := filepath.Join(t.TempDir(), "cancel-exit.wasm")
	//nolint:gosec // Fixed pinned executable and package; output is the test-owned temporary directory.
	build := exec.CommandContext(t.Context(), "go", "build", "-o", wasm, "./testdata/cancel-exit")

	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile actual WASM cancellation fixture: %v\n%s", err, output)
	}

	root, err := exec.CommandContext(t.Context(), "go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal("locate matching compiler bridge", err)
	}

	bridge := filepath.Join(strings.TrimSpace(string(root)), "lib", "wasm", "wasm_exec.js")

	for _, phase := range []string{"fetch", "digest"} {
		for _, outcome := range []string{"resolve", "reject"} {
			t.Run(phase+"/"+outcome, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), 9*time.Second)
				defer cancel()

				//nolint:gosec // Fixed Node/runner; paths are the selected compiler and test-owned output, with constant case arguments.
				command := exec.CommandContext(
					ctx,
					"node",
					"--stack-size=8192",
					"testdata/cancel-exit/runner.cjs",
					bridge,
					wasm,
					phase,
					outcome,
				)

				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("late %s %s called exited WASM: %v\n%s", phase, outcome, err, output)
				}

				if !bytes.Contains(output, []byte("late settlement after WASM exit: PASS")) {
					t.Fatalf("late %s %s did not execute its completion assertions:\n%s", phase, outcome, output)
				}
			})
		}
	}
}
