//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type outputValue struct {
	data []byte
	hash string
}

type fileValue struct {
	data []byte
	hash string
	err  error
}

type runner struct {
	pipelines  []pipelineDefinition
	jobs       int
	trace      func(ActionEvent)
	snapshotMu *sync.Mutex
	project    string
	output     string
	cache      string
	buildState string
	identity   string
	noCache    bool
	log        io.Writer
	actions    map[string]Action
	outputs    map[Ref]outputValue
	files      map[string]fileValue
	state      map[string]uint8
	artifacts  map[string]map[Artifact]bool
	report     Report
}

// Build plans and executes a project pipeline, then publishes all generated outputs.
// Failed processing never changes the previously published generated tree.
func Build(ctx context.Context, config Config, options Options) (report Report, err error) {
	start := time.Now()

	ctx, closeCodecs := buildCodecContext(ctx)
	defer func() { err = errors.Join(err, closeCodecs()) }()

	run, err := newRunner(config, options)
	if err != nil {
		return Report{}, err
	}

	unlock, err := acquireLock(ctx, run.project)
	if err != nil {
		return Report{}, err
	}
	defer unlock()

	if err := run.recoverPublication(); err != nil {
		return Report{}, err
	}

	if _, err := run.oldOwnership(); err != nil {
		return Report{}, err
	}

	plan, err := run.makePlan(config, options)
	if plan != nil {
		defer func() {
			if cleanupErr := closePlanCleanups(plan); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}()
	}

	if err != nil {
		return Report{}, err
	}

	for _, action := range plan.actions {
		run.actions[action.ID] = action
	}

	if err := run.executeConfiguredPlan(ctx, plan); err != nil {
		return run.report, err
	}

	if err := ctx.Err(); err != nil {
		return run.report, err
	}

	bundle, err := run.collectConfiguredPlan(plan)
	if err != nil {
		return run.report, err
	}

	if err := run.publish(bundle); err != nil {
		return run.report, err
	}

	run.report.Duration = time.Since(start)
	if run.log != nil {
		_, _ = fmt.Fprintf(run.log, "Processing: %d executed, %d reused; published: %d replaced, %d unchanged, %d removed\n",
			run.report.Executed, run.report.Reused, run.report.Written, run.report.Unchanged, run.report.Removed)
	}

	return run.report, nil
}

func newRunner(config Config, options Options) (*runner, error) {
	if options.Phase != "" && options.Phase != PhaseGenerate && options.Phase != PhaseBuild {
		return nil, fmt.Errorf("unsupported asset phase %q: %w", options.Phase, errPlan)
	}

	if options.Jobs < 0 || options.Jobs > maxPlanSteps {
		return nil, fmt.Errorf("asset job count %d outside 0..%d: %w", options.Jobs, maxPlanSteps, errPlan)
	}

	pipelines, err := normalizePipelines(config)
	if err != nil {
		return nil, err
	}

	project, err := projectRoot(options.Project)
	if err != nil {
		return nil, err
	}

	output, err := confined(project, config.Output)
	if err != nil || config.Output == "" || output == project {
		return nil, fmt.Errorf("generated output must be inside project: %w", errPath)
	}

	buildState, err := confined(project, stateDirectory)
	if err != nil {
		return nil, err
	}

	cache, err := confined(project, cacheDirectory)
	if err != nil {
		return nil, err
	}

	if containsPath(buildState, output) {
		return nil, fmt.Errorf("generated output overlaps build state: %w", errPath)
	}

	if err := validateConfiguredPaths(project, output, buildState, config); err != nil {
		return nil, err
	}

	identity := options.Identity
	if identity == "" {
		identity, err = ExecutableIdentity()
		if err != nil {
			return nil, err
		}
	}

	var snapshotMu *sync.Mutex
	if options.Jobs > 1 {
		snapshotMu = &sync.Mutex{}
	}

	return &runner{
		pipelines: pipelines,
		jobs:      options.Jobs, trace: options.Trace, snapshotMu: snapshotMu,
		project:    project,
		output:     output,
		cache:      cache,
		buildState: buildState,
		identity:   identity,
		noCache:    options.NoCache,
		log:        options.Log,
		actions:    make(map[string]Action),
		outputs:    make(map[Ref]outputValue),
		files:      make(map[string]fileValue),
		state:      make(map[string]uint8),
		artifacts:  make(map[string]map[Artifact]bool),
	}, nil
}

// ExecutableIdentity fingerprints the actual recipe binary, including local code changes.
func ExecutableIdentity() (string, error) {
	name, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate recipe executable: %w", err)
	}

	data, err := readBounded(name)
	if err != nil {
		return "", fmt.Errorf("fingerprint recipe: %w", err)
	}

	return digest(data), nil
}

func (run *runner) makePlan(config Config, options Options) (*Plan, error) {
	profile := options.Profile
	if profile == "" {
		profile = "dev"
	}

	profiles := config.Profiles
	if len(profiles) == 0 {
		profiles = []string{"dev", "release"}
	}

	if !slices.Contains(profiles, profile) {
		return nil, fmt.Errorf("unsupported asset profile %q: %w", profile, errPlan)
	}

	environment := Environment{Phase: options.Phase, Profile: profile, TargetOS: options.TargetOS, TargetArch: options.TargetArch}
	if environment.Phase == "" {
		environment.Phase = PhaseBuild
	}

	if environment.TargetOS == "" {
		environment.TargetOS = runtime.GOOS
	}

	if environment.TargetArch == "" {
		environment.TargetArch = runtime.GOARCH
	}

	plan := &Plan{Project: run.project, Output: run.output, Environment: environment}

	return plan, planPipelines(plan, run.pipelines)
}

func validateSourceRoots(project, output, buildState string, patterns []string) error {
	for _, pattern := range patterns {
		prefix, err := globPrefix(pattern)
		if err != nil {
			return err
		}

		if prefix == "" && !strings.Contains(pattern, "**") {
			continue
		}

		root := project
		if prefix != "" {
			root, err = confined(project, prefix)
			if err != nil {
				return err
			}
		}

		if overlaps(root, output) || overlaps(root, buildState) {
			return fmt.Errorf("source %q overlaps generated output/build state: %w", pattern, errPath)
		}
	}

	return nil
}

const (
	actionVisiting uint8 = iota + 1
	actionDone
)

func (run *runner) process(ctx context.Context, action Action) error {
	base, err := run.actionBase(action)
	if err != nil {
		return err
	}

	result, reason, err := run.lookup(action, base)
	if err != nil {
		return err
	}

	record := ActionReport{ID: action.ID, Processor: action.Processor, Status: "HIT", Reason: reason}

	if result == nil {
		start := time.Now()

		result, err = run.runAction(ctx, action, base)
		if err != nil {
			return err
		}

		record.Status = "MISS"
		record.Duration = time.Since(start)
		run.report.Executed++
	} else {
		run.report.Reused++
	}

	for _, slot := range action.Outputs {
		data := slices.Clone(result[slot.Name])

		hash, err := jsonDigest([]any{slot.Schema, digest(data)})
		if err != nil {
			return err
		}

		run.outputs[Ref{Action: action.ID, Slot: slot.Name}] = outputValue{data: data, hash: hash}
	}

	run.report.Actions = append(run.report.Actions, record)
	if run.log != nil {
		_, _ = fmt.Fprintf(run.log, "%s: %s %s (%s)\n", record.Processor, record.ID, record.Status, record.Reason)
	}

	return nil
}

func (run *runner) runAction(ctx context.Context, action Action, base string) (Result, error) {
	actionContext := &Context{Context: ctx, runner: run, action: action}
	if err := run.admitDeclaredArtifacts(ctx, actionContext); err != nil {
		return nil, err
	}

	result, err := action.Run(actionContext)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := validateResult(action, result); err != nil {
		return nil, err
	}

	run.rememberArtifacts(action.ID, actionContext.deps)

	if !run.noCache && action.Cache == CacheAutomatic {
		if err := run.store(base, result, actionContext.deps); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (run *runner) lookup(action Action, base string) (Result, string, error) {
	if run.noCache || action.Cache == CacheDisabled {
		return nil, "cache disabled", nil
	}

	result, deps, reason, err := run.cached(action, base)
	if result != nil && err == nil {
		run.rememberArtifacts(action.ID, deps)
	}

	return result, reason, err
}

func validateResult(action Action, result Result) error {
	if len(result) != len(action.Outputs) {
		return fmt.Errorf("action %s output roster differs from declaration: %w", action.ID, errPlan)
	}

	for _, slot := range action.Outputs {
		data, exists := result[slot.Name]
		if !exists || len(data) > maxArtifactBytes {
			return fmt.Errorf("action %s missing/oversized output %q: %w", action.ID, slot.Name, errPlan)
		}
	}

	return nil
}

func (run *runner) collect(refs []Ref) (Bundle, error) {
	bundle := make(Bundle)

	for _, ref := range refs {
		value, exists := run.outputs[ref]
		if !exists {
			return nil, fmt.Errorf("published slot %+v is missing: %w", ref, errPlan)
		}

		files, err := decodeBundle(value.data)
		if err != nil {
			return nil, fmt.Errorf("decode generated bundle %+v: %w", ref, err)
		}

		for name, data := range files {
			if _, exists := bundle[name]; exists {
				return nil, fmt.Errorf("duplicate output owner %q: %w", name, errPlan)
			}

			bundle[name] = data
		}
	}

	return bundle, nil
}
