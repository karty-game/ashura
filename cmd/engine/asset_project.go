//go:build !js

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

const assetConfigFile = "asset.config.go"

var errAssetProject = errors.New("invalid asset project directory or target")

type assetOptions struct {
	phase   assetbuild.Phase
	profile string
	target  string
	noCache bool
	jobs    int
	log     io.Writer
}

func prepareProjectAssets(ctx context.Context, project string, options assetOptions) error {
	project, err := filepath.Abs(project)
	if err != nil {
		return fmt.Errorf("asset project: %w", err)
	}

	info, err := os.Lstat(project)
	if err != nil {
		return fmt.Errorf("asset project: %w", err)
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errAssetProject
	}

	canonical, err := filepath.EvalSymlinks(project)
	if err != nil || canonical != project {
		return fmt.Errorf("asset project must not follow symlinks: %w", errAssetProject)
	}

	options, err = normalizeAssetOptions(options)
	if err != nil {
		return err
	}

	config := filepath.Join(project, assetConfigFile)

	info, err = os.Lstat(config)
	if errors.Is(err, os.ErrNotExist) {
		return prepareDefaultProject(ctx, project, options)
	}

	if err != nil {
		return fmt.Errorf("asset config: %w", err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("asset config must be a regular file: %w", errAssetProject)
	}

	return executeAssetRecipe(ctx, project, options)
}

func normalizeAssetOptions(options assetOptions) (assetOptions, error) {
	if options.phase == "" {
		options.phase = assetbuild.PhaseBuild
	}

	if options.phase != assetbuild.PhaseGenerate && options.phase != assetbuild.PhaseBuild {
		return assetOptions{}, errAssetProject
	}

	if options.jobs < 0 {
		return assetOptions{}, errAssetProject
	}

	if options.log == nil {
		options.log = os.Stdout
	}

	if options.profile == "" {
		options.profile = "dev"
	}

	if options.target == "" {
		options.target = runtime.GOOS + "/" + runtime.GOARCH
	}

	parts := strings.Split(options.target, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return assetOptions{}, errAssetProject
	}

	return options, nil
}

func prepareDefaultProject(ctx context.Context, project string, options assetOptions) error {
	info, err := os.Lstat(filepath.Join(project, "assets"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("asset sources: %w", err)
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("asset sources must be a directory: %w", errAssetProject)
	}

	parts := strings.Split(options.target, "/")
	_, err = assetbuild.Build(ctx, assetbuild.DefaultConfig([]string{"assets/**"}, "generated/assets"), assetbuild.Options{
		Phase:   options.phase,
		Project: project, Profile: options.profile, TargetOS: parts[0], TargetArch: parts[1],
		NoCache: options.noCache, Log: options.log, Jobs: options.jobs,
	})

	return err
}

func executeAssetRecipe(ctx context.Context, project string, options assetOptions) error {
	arguments := []string{
		"run", "-trimpath", "-tags=nofakecgo", assetConfigFile,
		"--protocol=" + strconv.Itoa(assetbuild.InvocationVersion), "--project=" + project,
		"--phase=" + string(options.phase),
		"--profile=" + options.profile, "--target=" + options.target,
	}
	if options.noCache {
		arguments = append(arguments, "--no-cache")
	}

	if options.jobs > 0 {
		arguments = append(arguments, "--jobs="+strconv.Itoa(options.jobs))
	}

	// go run caches the linked executable as well as compiled packages. Building
	// into a fresh temporary directory forced every warm invocation to relink.
	//nolint:gosec // Fixed Go arguments and recipe flags are never interpreted by a shell.
	command := exec.CommandContext(ctx, "go", arguments...)
	command.Dir, command.Env = project, hostBuildEnvironment()
	configureRecipeProcess(command)

	var diagnostics assetRecipeStderr

	command.Stdout = options.log

	command.Stderr = io.MultiWriter(&diagnostics, os.Stderr)
	if err := command.Run(); err != nil {
		if detail := strings.TrimSpace(diagnostics.String()); detail != "" {
			return fmt.Errorf("execute %s: %w\nrecipe stderr:\n%s", filepath.Join(project, assetConfigFile), err, detail)
		}

		return fmt.Errorf("execute %s: %w", filepath.Join(project, assetConfigFile), err)
	}

	return nil
}

func hostBuildEnvironment() []string {
	var environment []string

	for _, value := range os.Environ() {
		name, setting, _ := strings.Cut(value, "=")
		switch name {
		case "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOWASM", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64":
			continue
		case "PATH":
			environment = append(environment, "PATH="+uniqueSearchPath(setting))
		default:
			environment = append(environment, value)
		}
	}

	return append(environment, "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
}

// Go generate and tool wrappers prepend tool directories to PATH. Duplicates
// do not change tool lookup, but would invalidate tracked PATH dependencies.
func uniqueSearchPath(value string) string {
	separator := string(os.PathListSeparator)
	parts := strings.Split(value, separator)
	unique := parts[:0]

	for _, part := range parts {
		if !slices.Contains(unique, part) {
			unique = append(unique, part)
		}
	}

	return strings.Join(unique, separator)
}
