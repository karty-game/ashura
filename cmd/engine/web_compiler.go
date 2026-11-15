//go:build !js

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	tinyGoVersion  = "0.44.0"
	tinyGoCompiler = "tinygo"
)

// Resolve the compiler and its matching JS bridge together. Compiler caches are
// owned by Go/TinyGo; asset action identities remain independent of this choice.
type webCompiler struct {
	Name          string   `json:"compiler"`
	Version       string   `json:"version"`
	Flags         []string `json:"flags"`
	RuntimeSHA256 string   `json:"runtimeSha256"`
	executable    string
	toolRoot      string
	runtime       []byte
}

func resolveWebCompiler(ctx context.Context, name string) (webCompiler, error) {
	if name == "" {
		name = "go"
	}

	if name != "go" && name != tinyGoCompiler {
		return webCompiler{}, fmt.Errorf("unsupported browser compiler %q: %w", name, errUsage)
	}

	executable, err := exec.LookPath(name)
	if err != nil {
		return webCompiler{}, fmt.Errorf("locate %s browser compiler (run mise install): %w", name, err)
	}

	//nolint:gosec // Executable is the explicitly selected Go/TinyGo found on PATH.
	version, err := exec.CommandContext(ctx, executable, "version").Output()
	if err != nil {
		return webCompiler{}, fmt.Errorf("read %s version: %w", name, err)
	}

	compiler := webCompiler{Name: name, Version: strings.TrimSpace(string(version)), executable: executable}
	rootVariable := "GOROOT"

	if name == tinyGoCompiler {
		if !strings.HasPrefix(compiler.Version, "tinygo version "+tinyGoVersion+" ") {
			return webCompiler{}, fmt.Errorf("browser builds require TinyGo %s; found %s: %w", tinyGoVersion, compiler.Version, errUsage)
		}

		rootVariable = "TINYGOROOT"
		compiler.Flags = []string{"-target=wasm", "-opt=z", "-no-debug"}
		// Isolated fixtures may opt out of VCS stamping. This flag is also
		// supported by TinyGo; do not forward arbitrary standard Go flags.
		if slices.Contains(strings.Fields(os.Getenv("GOFLAGS")), "-buildvcs=false") {
			compiler.Flags = append(compiler.Flags, "-buildvcs=false")
		}
	}

	//nolint:gosec // The selected compiler reports its installation root; rootVariable is fixed above.
	root, err := exec.CommandContext(ctx, executable, "env", rootVariable).Output()
	if err != nil {
		return webCompiler{}, fmt.Errorf("locate %s root: %w", name, err)
	}

	compiler.toolRoot = strings.TrimSpace(string(root))

	compiler.runtime, err = readWasmRuntime(compiler.toolRoot, name)
	if err != nil {
		return webCompiler{}, err
	}

	digest := sha256.Sum256(compiler.runtime)
	compiler.RuntimeSHA256 = hex.EncodeToString(digest[:])

	return compiler, nil
}

func readWasmRuntime(root, compiler string) ([]byte, error) {
	candidates := []string{filepath.Join(root, "lib", "wasm", wasmExecFile), filepath.Join(root, "misc", "wasm", wasmExecFile)}
	if compiler == tinyGoCompiler {
		candidates = []string{filepath.Join(root, "targets", wasmExecFile)}
	}

	for _, source := range candidates {
		//nolint:gosec // The selected compiler reports its own installation root.
		contents, err := os.ReadFile(source)
		if err == nil {
			return contents, nil
		}
	}

	return nil, fmt.Errorf("%w: %s (%s)", errWasmRuntimeNotFound, root, compiler)
}

func (compiler webCompiler) stageRuntime(dist string) error {
	//nolint:gosec // dist is the CLI's fresh distribution staging directory.
	if err := os.WriteFile(filepath.Join(dist, wasmExecFile), compiler.runtime, 0o600); err != nil {
		return fmt.Errorf("stage %s WASM runtime: %w", compiler.Name, err)
	}

	identity, err := json.MarshalIndent(compiler, "", "  ")
	if err != nil {
		return fmt.Errorf("encode browser compiler identity: %w", err)
	}

	//nolint:gosec // The fixed receipt belongs to this fresh distribution only.
	if err := os.WriteFile(filepath.Join(dist, "build-info.json"), append(identity, '\n'), 0o600); err != nil {
		return fmt.Errorf("write browser compiler identity: %w", err)
	}

	return nil
}

func (compiler webCompiler) build(ctx context.Context, root, dist, packagePath string) error {
	arguments := append([]string{"build"}, compiler.Flags...)
	arguments = append(arguments, "-o", filepath.Join(dist, wasmFile), packagePath)
	//nolint:gosec // Executable was resolved once; packagePath is one argv value, never shell code.
	command := exec.CommandContext(ctx, compiler.executable, arguments...)
	command.Dir = root
	command.Env = webBuildEnvironment(compiler.Name)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("build %s for browser with %s: %w", packagePath, compiler.Name, err)
	}

	return nil
}

func webBuildEnvironment(compiler string) []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GOOS", "GOARCH", "CGO_ENABLED":
			continue
		case "GOFLAGS":
			// Standard Go flags are not a TinyGo command-line contract.
			if compiler == tinyGoCompiler {
				// TinyGo's loader invokes go list as well as its own VCS
				// handling, so isolated fixtures need this on both paths.
				if slices.Contains(strings.Fields(os.Getenv("GOFLAGS")), "-buildvcs=false") {
					environment = append(environment, "GOFLAGS=-buildvcs=false")
				}

				continue
			}
		}

		environment = append(environment, entry)
	}

	return append(environment, "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
}
