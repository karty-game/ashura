//go:build !js

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

var errInspectArgs = errors.New(
	"usage: engine assets inspect [directory | --project directory] [--phase generate|build] [--profile dev] [--target os/arch]",
)

func inspectAssets(ctx context.Context, args []string, out io.Writer) error {
	// Accept the project first as well as the Go flag package's usual flags-first syntax.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append(append([]string(nil), args[1:]...), args[0])
	}

	flags := flag.NewFlagSet("assets inspect", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(out, errInspectArgs)

		flags.PrintDefaults()
	}

	project := flags.String("project", "", "project directory (defaults to the current directory)")
	phase := flags.String("phase", string(assetbuild.PhaseGenerate), "recipe lifecycle to inspect")
	profile := flags.String("profile", "dev", "recipe profile")
	target := flags.String("target", "", "resource target OS/architecture")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	projectSet := false

	flags.Visit(func(value *flag.Flag) {
		projectSet = projectSet || value.Name == "project"
	})

	if flags.NArg() > 1 || (projectSet && (flags.NArg() != 0 || *project == "")) ||
		(*phase != string(assetbuild.PhaseGenerate) && *phase != string(assetbuild.PhaseBuild)) || *profile == "" {
		return errInspectArgs
	}

	if flags.NArg() == 1 {
		*project = flags.Arg(0)
	} else if !projectSet {
		*project = "."
	}

	return prepareProjectAssets(ctx, *project, assetOptions{
		phase: assetbuild.Phase(*phase), profile: *profile, target: *target, inspect: true, log: out,
	})
}
