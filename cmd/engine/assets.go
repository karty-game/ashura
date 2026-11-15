//go:build !js

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

var errTextureArgs = errors.New("usage: engine assets texture --input source.png --output prepared.ktx2 --usage albedo|normal|ao|ssbump")

func buildTexture(args []string) error {
	flags := flag.NewFlagSet("assets texture", flag.ContinueOnError)
	input := flags.String("input", "", "explicit source PNG")
	output := flags.String("output", "", "prepared KTX2 output")

	usage := flags.String("usage", "", "albedo, normal, ao or ssbump")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 || *input == "" || *output == "" || *usage == "" {
		return errTextureArgs
	}

	result, err := assetbuild.PrepareTexture(*input, *output, *usage)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(
		os.Stdout,
		"Prepared %s -> %s (%dx%d, %d mips, %s, %d bytes)\n",
		*input,
		*output,
		result.Width,
		result.Height,
		result.MipLevels,
		*usage,
		result.Bytes,
	)

	return nil
}

var errCatalogArgs = errors.New(
	"usage: engine assets generate|build [go-package-or-pattern | --project directory | --source assets --output generated/assets] [--profile dev] [--no-cache] [--jobs count]",
)

func buildAssetCatalog(ctx context.Context, args []string) error {
	return runAssetCatalog(ctx, args, assetbuild.PhaseBuild)
}

func runAssetCatalog(ctx context.Context, args []string, phase assetbuild.Phase) error {
	// Allow the common `assets generate ./samples/... --profile dev` spelling.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append(append([]string(nil), args[1:]...), args[0])
	}

	flags := flag.NewFlagSet("assets "+string(phase), flag.ContinueOnError)
	source := flags.String("source", "assets", "source PNG/JPG directory")

	output := flags.String("output", "generated/assets", "generated package and KTX2 directory")
	project := flags.String("project", "", "project directory containing asset.config.go")
	profile := flags.String("profile", "dev", "processing profile")
	target := flags.String("target", "", "resource target OS/architecture")
	jobs := flags.Int("jobs", 0, "maximum independent build actions; zero or one keeps serial execution")

	noCache := flags.Bool("no-cache", false, "execute without the persistent action cache")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() > 1 || *source == "" || *output == "" || *profile == "" || *jobs < 0 {
		return errCatalogArgs
	}

	selected := make(map[string]bool)

	flags.Visit(func(value *flag.Flag) { selected[value.Name] = true })

	options, err := normalizeAssetOptions(assetOptions{phase: phase, profile: *profile, target: *target, noCache: *noCache, jobs: *jobs})
	if err != nil {
		return err
	}

	if flags.NArg() == 1 {
		if selected["project"] || selected["source"] || selected["output"] {
			return errCatalogArgs
		}

		return prepareAssetPattern(ctx, flags.Arg(0), options)
	}

	if selected["project"] {
		if *project == "" || selected["source"] || selected["output"] {
			return errCatalogArgs
		}

		return prepareProjectAssets(ctx, *project, options)
	}

	if !selected["source"] && !selected["output"] {
		return prepareProjectAssets(ctx, ".", options)
	}

	parts := strings.Split(options.target, "/")
	_, err = assetbuild.BuildDirectory(ctx, *source, *output, assetbuild.Options{
		Phase:   phase,
		Profile: options.profile, TargetOS: parts[0], TargetArch: parts[1], NoCache: options.noCache, Log: os.Stdout, Jobs: options.jobs,
	})

	return err
}

func prepareAssetPattern(ctx context.Context, pattern string, options assetOptions) error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}

	packages, err := listWebPackages(ctx, root, pattern)
	if err != nil {
		return err
	}

	return prepareWebAssets(ctx, root, packages, options)
}
