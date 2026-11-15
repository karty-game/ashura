//go:build !js

package assetbuild

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
)

const (
	materializeToolVersion = "2.0.0"
	materializeToolName    = "materialize"
)

// Pin the existing published release. Its Windows package requires the Visual
// C++ runtime; this prerequisite does not justify selecting an unpublished patch.
//
//go:embed tool_releases/materialize.json
var materializeReleaseJSON []byte

func materializeToolSpecification(goos, goarch string) (ManagedToolSpec, error) {
	var release managedToolRelease
	if err := json.Unmarshal(materializeReleaseJSON, &release); err != nil {
		return ManagedToolSpec{}, fmt.Errorf("Materialize release manifest: %w", err)
	}

	return release.forTarget(goos + "-" + goarch)
}

func materializeExecutable(ctx *Context, options MaterializeOptions) (string, error) {
	name := options.Executable
	if name == "" {
		name = ctx.Getenv("MATERIALIZE_CLI")
	}

	if name != "" {
		return ctx.TrackTool(name)
	}

	spec, err := materializeToolSpecification(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}

	installed, err := ctx.ManagedTool(spec)

	return installed.Executable, err
}

// Include the module-owned artifact in action identity even when a caller
// supplies an explicit build identity. Tool and environment dependencies still
// invalidate actual selected executable changes independently.
func materializeActionOptions(options MaterializeOptions) any {
	var managed *ManagedToolSpec

	if options.Executable == "" {
		if spec, err := materializeToolSpecification(runtime.GOOS, runtime.GOARCH); err == nil {
			managed = &spec
		}
	}

	return struct {
		Settings MaterializeOptions `json:"settings"`
		Managed  *ManagedToolSpec   `json:"managed,omitempty"`
	}{Settings: options, Managed: managed}
}
