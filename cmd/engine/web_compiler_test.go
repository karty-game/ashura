//go:build !js

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTinyGoHost(t *testing.T) {
	t.Parallel()

	if expected := os.Getenv("ASHURA_TEST_HOST"); expected != "" && expected != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("expected compiler host %s; executing on %s/%s", expected, runtime.GOOS, runtime.GOARCH)
	}
}

func TestParseWebOptions(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"./samples/...", "--compiler=tinygo", "--profile=release"},
		{"--compiler", "tinygo", "./samples/...", "--profile", "release"},
	} {
		remaining, options, err := parseWebOptions(args)
		if err != nil || options.compiler != "tinygo" || options.assets.profile != "release" ||
			!slices.Equal(remaining, []string{"./samples/..."}) {
			t.Fatalf("parse %v: %v %+v %v", args, remaining, options, err)
		}
	}

	_, options, err := parseWebOptions(nil)
	if err != nil || options.compiler != "go" {
		t.Fatalf("default compiler: %+v %v", options, err)
	}

	for _, args := range [][]string{{"--compiler"}, {"--compiler="}, {"--compiler=unknown"}, {"--compiler", "--output"}} {
		if _, _, err := parseWebOptions(args); !errors.Is(err, errUsage) {
			t.Fatalf("accepted invalid compiler %v: %v", args, err)
		}
	}
}

func TestTinyGoEnvironmentKeepsHostFlagsOut(t *testing.T) {
	t.Setenv("GOFLAGS", "-race -tags=nofakecgo")
	t.Setenv("GOOS", "linux")
	t.Setenv("GOARCH", "arm64")
	t.Setenv("CGO_ENABLED", "1")

	environment := webBuildEnvironment("tinygo")
	for _, entry := range environment {
		if strings.HasPrefix(entry, "GOFLAGS=") {
			t.Fatalf("TinyGo inherited Go flags: %s", entry)
		}
	}

	for _, entry := range []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"} {
		if !slices.Contains(environment, entry) {
			t.Fatalf("missing target %s: %v", entry, environment)
		}
	}

	if !slices.Contains(webBuildEnvironment("go"), "GOFLAGS=-race -tags=nofakecgo") {
		t.Fatal("standard Go lost caller flags")
	}
}

func TestWasmRuntimeFollowsSelectedCompiler(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWebFixtures(t, root, map[string]string{
		"lib/wasm/wasm_exec.js": "standard bridge",
		"targets/wasm_exec.js":  "TinyGo bridge",
	})

	for compiler, expected := range map[string]string{"go": "standard bridge", "tinygo": "TinyGo bridge"} {
		bridge, err := readWasmRuntime(root, compiler)
		if err != nil || string(bridge) != expected {
			t.Fatalf("%s runtime: %q %v", compiler, bridge, err)
		}
	}

	if err := os.Remove(filepath.Join(root, "targets", wasmExecFile)); err != nil {
		t.Fatal(err)
	}

	if _, err := readWasmRuntime(root, "tinygo"); !errors.Is(err, errWasmRuntimeNotFound) {
		t.Fatalf("TinyGo fell back to standard Go's incompatible bridge: %v", err)
	}
}

func TestTinyGoWebExport(t *testing.T) {
	// Discovery uses standard Go in an isolated fixture without a VCS checkout.
	t.Setenv("GOFLAGS", "-buildvcs=false")

	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("TinyGo is optional for standard Go users; mise install supplies the pinned compiler")
	}

	compiler, err := resolveWebCompiler(t.Context(), "tinygo")
	if err != nil {
		t.Fatal(err)
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	writeWebFixtures(t, root, map[string]string{
		"go.mod":                            "module example.test/tinygo\n\ngo 1.27.0\n",
		"a/view/main.go":                    tinyGoAsyncFixture,
		"b/view/main.go":                    "package main\nfunc main() { println(\"two\") }\n",
		"b/view/generated/public/asset.bin": "prepared resource",
	})

	outputRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(outputRoot, "site")

	options := webOptions{compiler: "tinygo"}
	if err := buildWebDistribution(t.Context(), root, output, "./...", options); err != nil {
		t.Fatal(err)
	}

	assertTinyGoDistribution(t, output, compiler)
	assertTinyGoExecution(t, output, compiler)
	writeWebFixtures(t, root, map[string]string{"b/view/main.go": "package main\nfunc main() { missing() }\n"})

	failed := filepath.Join(filepath.Dir(output), "failed")
	if err := buildWebDistribution(t.Context(), root, failed, "./...", options); err == nil {
		t.Fatal("TinyGo published a failed gallery")
	}

	entries, err := os.ReadDir(filepath.Dir(output))
	if err != nil || len(entries) != 1 || entries[0].Name() != "site" {
		t.Fatalf("failed TinyGo export left partial output: %v, %v", entries, err)
	}
}

func assertTinyGoExecution(t *testing.T, output string, compiler webCompiler) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	runner := filepath.Join(compiler.toolRoot, "targets", "wasm_exec_node.js")
	module := filepath.Join(output, "apps", "a", "view", wasmFile)
	//nolint:gosec // Fixed upstream runner and private fixture WASM, passed directly to pinned Node.
	result, err := exec.CommandContext(ctx, "node", runner, module).CombinedOutput()
	if err != nil || !bytes.Contains(result, []byte("async bridge ready")) {
		t.Fatalf("TinyGo asynchronous JS/goroutine execution: %s %v", result, err)
	}
}

const tinyGoAsyncFixture = `package main
import ("syscall/js"; "time")
func main() {
    done := make(chan string, 1)
    callback := js.FuncOf(func(_ js.Value, args []js.Value) any {
        done <- args[0].String()
        return nil
    })
    defer callback.Release()
    js.Global().Get("Promise").Call("resolve", "async bridge ready").Call("then", callback)
    value := <-done
    <-time.After(time.Millisecond)
    println(value)
}
`

func assertTinyGoDistribution(t *testing.T, output string, compiler webCompiler) {
	t.Helper()

	//nolint:gosec // Fixed paths beneath the test's private export.
	bridge, err := os.ReadFile(filepath.Join(output, wasmExecFile))
	if err != nil || !bytes.Equal(bridge, compiler.runtime) {
		t.Fatalf("TinyGo binary received a different runtime bridge: %v", err)
	}

	//nolint:gosec // Fixed paths beneath the test's private export.
	receipt, err := os.ReadFile(filepath.Join(output, "build-info.json"))
	if err != nil {
		t.Fatal(err)
	}

	var identity webCompiler
	if err := json.Unmarshal(receipt, &identity); err != nil || identity.Name != "tinygo" ||
		identity.RuntimeSHA256 != compiler.RuntimeSHA256 || identity.Version != compiler.Version {
		t.Fatalf("wrong compiler identity: %s %v", receipt, err)
	}

	for _, sample := range []string{"a/view", "b/view"} {
		//nolint:gosec // Fixed sample paths beneath the test's private export.
		binary, err := os.ReadFile(filepath.Join(output, "apps", sample, wasmFile))
		if err != nil || !bytes.HasPrefix(binary, []byte("\x00asm")) {
			t.Fatalf("invalid TinyGo module for %s: %v", sample, err)
		}
	}

	//nolint:gosec // Fixed resource path beneath the test's private export.
	resource, err := os.ReadFile(filepath.Join(output, "apps/b/view/asset.bin"))
	if err != nil || string(resource) != "prepared resource" {
		t.Fatalf("TinyGo export lost its prepared resource: %q %v", resource, err)
	}
}
