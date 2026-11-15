//go:build !js

package assetbuild

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	managedMiseTimeout   = 2 * time.Minute
	managedMiseWaitDelay = 2 * time.Second
	managedMiseLogLimit  = 64 << 10
)

func installMiseTool(ctx context.Context, project string, spec ManagedToolSpec, mise string) (ManagedTool, error) {
	argument, err := managedMiseArgument(spec)
	if err != nil {
		return ManagedTool{}, err
	}

	if _, err := runManagedMise(ctx, project, mise, "install", argument); err != nil {
		return ManagedTool{}, err
	}

	output, err := runManagedMise(ctx, project, mise, "where", argument)
	if err != nil {
		return ManagedTool{}, err
	}

	directory := strings.TrimSpace(output)
	if !filepath.IsAbs(directory) || len(directory) > managedPathLimit || strings.ContainsAny(directory, "\x00\r\n") {
		return ManagedTool{}, fmt.Errorf("mise where returned an invalid installation directory: %w", errManagedTool)
	}

	installed := ManagedTool{}

	installed.Executable, err = managedInstalledPath(directory, spec.Executable)
	if err != nil {
		return ManagedTool{}, err
	}

	for _, component := range spec.Components {
		full, err := managedInstalledPath(directory, component)
		if err != nil {
			return ManagedTool{}, err
		}

		installed.Components = append(installed.Components, full)
	}

	return installed, nil
}

func runManagedMise(ctx context.Context, project, mise, operation, argument string) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, managedMiseTimeout)
	defer cancel()

	//nolint:gosec // TrackTool fingerprints mise; fixed HTTP backend arguments never pass through a shell or project hooks.
	command := exec.CommandContext(bounded, mise, "--no-config", "--no-env", "--no-hooks", operation, argument)
	command.Dir, command.WaitDelay = project, managedMiseWaitDelay
	stdout, stderr := &managedMiseOutput{}, &managedMiseOutput{}
	command.Stdout, command.Stderr = stdout, stderr

	if err := command.Run(); err != nil {
		if bounded.Err() != nil {
			return "", fmt.Errorf("mise %s stopped: %w", operation, bounded.Err())
		}

		return "", fmt.Errorf("mise >=2026.11.5 %s failed (%s%s): %w", operation, stdout.String(), stderr.String(), err)
	}

	return stdout.String(), nil
}

type managedMiseOutput struct {
	mu   sync.Mutex
	data []byte
}

func (output *managedMiseOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()

	remaining := managedMiseLogLimit - len(output.data)
	output.data = append(output.data, data[:min(len(data), remaining)]...)

	return len(data), nil
}

func (output *managedMiseOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()

	return string(output.data)
}
