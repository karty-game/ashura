//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
)

// InvocationVersion is the CLI-to-recipe protocol understood by Main.
const InvocationVersion = 2

var errInvocation = errors.New("invalid asset recipe invocation; use engine assets with --project <directory>")

// Main is the host entry point for an asset.config.go program. The CLI supplies
// the versioned invocation; recipe configuration never enters game executables.
func Main(recipe func(Environment) Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	_, err := RunRecipe(ctx, os.Args[1:], recipe)

	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Run starts an asset.config.go recipe with the same configuration in every build environment.
func Run(config Config) {
	Main(func(Environment) Config { return config })
}

// RunRecipe invokes the recipe entry point without exiting the process.
func RunRecipe(ctx context.Context, args []string, recipe func(Environment) Config) (Report, error) {
	invocation, err := recipeOptions(args)
	if err != nil {
		return Report{}, err
	}

	if recipe == nil {
		return Report{}, errInvocation
	}

	options := invocation.options
	env := Environment{Phase: options.Phase, Profile: options.Profile, TargetOS: options.TargetOS, TargetArch: options.TargetArch}

	config := recipe(env)
	if invocation.inspect {
		return Report{}, Inspect(ctx, config, options)
	}

	return Build(ctx, config, options)
}

type recipeInvocation struct {
	options Options
	inspect bool
}

func recipeOptions(args []string) (recipeInvocation, error) {
	flags := flag.NewFlagSet("asset recipe", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	version := flags.Int("protocol", 0, "CLI invocation protocol")
	phase := flags.String("phase", "", "generate or build lifecycle phase")
	project := flags.String("project", "", "project directory")
	profile := flags.String("profile", "dev", "processing profile")
	target := flags.String("target", runtime.GOOS+"/"+runtime.GOARCH, "resource target OS/architecture")

	inspect := flags.Bool("inspect", false, "show planning results without running actions")
	jobs := flags.Int("jobs", 1, "maximum independent build actions; defaults to serial")

	noCache := flags.Bool("no-cache", false, "execute without the persistent action cache")
	if err := flags.Parse(args); err != nil {
		return recipeInvocation{}, fmt.Errorf("asset recipe arguments: %w", err)
	}

	parts := strings.Split(*target, "/")
	// V1 callers retain their original build semantics. V2 makes the phase explicit.
	if *version == 1 && *phase == "" && !*inspect {
		*phase = string(PhaseBuild)
	} else if *version != InvocationVersion || (*phase != string(PhaseGenerate) && *phase != string(PhaseBuild)) {
		return recipeInvocation{}, errInvocation
	}

	if *project == "" || *profile == "" || flags.NArg() != 0 ||
		len(parts) != 2 || parts[0] == "" || parts[1] == "" || *jobs < 0 || *jobs > maxPlanSteps || (*inspect && *noCache) {
		return recipeInvocation{}, errInvocation
	}

	return recipeInvocation{inspect: *inspect, options: Options{
		Phase:   Phase(*phase),
		Project: *project, Profile: *profile, TargetOS: parts[0], TargetArch: parts[1],
		NoCache: *noCache, Log: os.Stdout, Jobs: *jobs,
	}}, nil
}
