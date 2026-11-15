//go:build !js

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func exportWeb(ctx context.Context, args []string) error {
	remaining, options, err := parseWebOptions(args)
	if err != nil {
		return err
	}

	pattern, output, err := parseWebExportArgs(remaining)
	if err != nil {
		return err
	}

	root, err := moduleRoot()
	if err != nil {
		return err
	}

	return buildWebDistribution(ctx, root, output, pattern, options)
}

func parseWebExportArgs(args []string) (string, string, error) {
	pattern, output := defaultPackage, "dist/samples"
	hasPackage := false

	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--output":
			if index+1 >= len(args) {
				return "", "", errUsage
			}

			index++

			output = args[index]
		case strings.HasPrefix(argument, "--output="):
			output = strings.TrimPrefix(argument, "--output=")
		default:
			if strings.HasPrefix(argument, "-") || hasPackage {
				return "", "", errUsage
			}

			pattern, hasPackage = argument, true
		}
	}

	if output == "" || strings.HasPrefix(output, "-") {
		return "", "", errUsage
	}

	return pattern, output, nil
}

// Build beside the destination and publish only after every sample succeeds.
// Existing output is refused so an export never deletes user files or old builds.
//
//nolint:gosec // The CLI intentionally accepts an output location; cleanup removes only its fresh MkdirTemp directory.
func buildWebDistribution(ctx context.Context, root, output, pattern string, options webOptions) error {
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("web output must not exist: %s: %w", output, os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect web output: %w", err)
	}

	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create web output parent: %w", err)
	}

	staging, err := os.MkdirTemp(parent, ".web-export-")
	if err != nil {
		return fmt.Errorf("create web staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	packages, err := listWebPackages(ctx, root, pattern)
	if err != nil {
		return err
	}

	if err := stageWebDistribution(ctx, root, staging, pattern, packages, options); err != nil {
		return err
	}

	log := options.assets.log
	if log == nil {
		log = os.Stdout
	}

	if err := checkWebSampleBudgets(staging, packages, log); err != nil {
		return err
	}

	if err := os.Rename(staging, output); err != nil {
		return fmt.Errorf("publish web distribution: %w", err)
	}

	return nil
}
