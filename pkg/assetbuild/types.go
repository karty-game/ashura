//go:build !js

// Package assetbuild prepares project assets offline through composable plugins.
// It is a host build-tool dependency and must not be imported by game runtimes.
package assetbuild

import (
	"context"
	"io"
	"time"
)

// Config selects project source inputs, processing plugins and generated output.
type Config struct {
	// Deprecated: Use Pipeline.Input in Pipelines.
	Input  []string
	Output string
	// Pipelines isolate source selection, resources and plugin graphs by name.
	// They cannot be combined with Input, Plugins or the shared plugin groups.
	Pipelines []Pipeline
	// Deprecated: Use Pipeline.Import in Pipelines.
	Import []Plugin
	// Deprecated: Use Pipeline.Process in Pipelines.
	Process []Plugin
	// Deprecated: Use Pipeline.Output in Pipelines.
	Export []Plugin
	// Deprecated: Use Pipelines with Import, Process and Output plugin lists.
	Plugins  []Plugin
	Profiles []string
}

// Pipeline plans one isolated asset flow beneath the shared Config.Output tree.
// Name must be a Go identifier and unique within the configuration.
type Pipeline struct {
	Name string
	// Directory optionally places published files below this relative subdirectory
	// of Config.Output. It does not change logical resource or action identities.
	Directory string
	Input     []string
	Import    []Plugin
	Process   []Plugin
	Output    []Plugin
}

// Phase identifies when a project recipe is being prepared.
type Phase string

const (
	// PhaseGenerate prepares declarations and assets before game development.
	PhaseGenerate Phase = "generate"
	// PhaseBuild refreshes prepared assets for a playable game build.
	PhaseBuild Phase = "build"
)

// Environment describes the selected lifecycle phase, profile and output target.
type Environment struct {
	Phase      Phase
	Profile    string
	TargetOS   string
	TargetArch string
}

// Options controls one build invocation. Identity defaults to the host executable hash.
type Options struct {
	// Phase defaults to PhaseBuild. Both phases use the same cached pipeline.
	Phase Phase
	// Jobs controls explicitly parallel-safe build actions. Zero and one retain
	// serial execution; larger values are bounded by the 65,536-node plan limit.
	Jobs int
	// Trace receives action lifecycle events synchronously on the coordinator.
	// It is optional, must return promptly and must not mutate the executing plan.
	Trace      func(ActionEvent) `json:"-"`
	Project    string
	Profile    string
	TargetOS   string
	TargetArch string
	Identity   string
	NoCache    bool
	Log        io.Writer
}

// Plugin adds processing actions to the current project plan.
type Plugin interface {
	Name() string
	Plan(plan *Plan) error
}

// CachePolicy selects automatic reuse or unconditional execution for an action.
type CachePolicy uint8

const (
	// CacheAutomatic reuses verified results when tracked inputs are unchanged.
	CacheAutomatic CachePolicy = iota
	// CacheDisabled executes every build while retaining content-based downstream reuse.
	CacheDisabled
)

// Ref identifies one immutable output slot of an action.
type Ref struct {
	Action string `json:"action"`
	Slot   string `json:"slot"`
}

// Slot declares an action output and its serialization schema.
type Slot struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
}

// Result contains serialized immutable outputs indexed by their declared slot name.
type Result map[string][]byte

// Bundle describes generated project-relative files beneath Config.Output.
// Encode it with EncodeBundle (or legacy JSON) and register its slot with Plan.Publish.
type Bundle map[string][]byte

// Action is one cacheable processor invocation. Options must be JSON-serializable.
type Action struct {
	namespace string
	// Parallel opts into independent execution when Options.Jobs exceeds one.
	// Its outputs must be independent of scheduling; own mutable state or serialize
	// access with Exclusive. Required processing order still uses input Refs.
	Parallel bool
	// Exclusive identifies one plugin-owned resource shared by parallel actions.
	// A nil token imposes no resource exclusion; serial actions remain barriers.
	Exclusive *ExecutionResource
	ID        string
	Processor string
	Revision  string
	Options   any
	Inputs    []Ref
	Artifacts []Artifact
	Outputs   []Slot
	Cache     CachePolicy
	Run       func(*Context) (Result, error)
}

// Expansion discovers bounded action membership from declared upstream manifests.
// Run receives a private child plan. Its graph is committed only after Run
// succeeds; created actions use the ordinary input/output cache and publication.
// Discovery may read declared inputs and their artifacts, never source files,
// tools or environment. Expensive processing belongs in cacheable actions.
type Expansion struct {
	ID     string
	Inputs []Ref
	Run    func(*Context, *Plan) error
}

// Artifact identifies an immutable auxiliary payload in the project build cache.
// It is useful when a small discovery manifest refers to many bounded payloads.
type Artifact struct {
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

// Resource associates a logical asset identity with its current prepared record.
type Resource struct {
	ID  string
	Ref Ref
}

// ActionReport describes actual action execution or cache reuse.
type ActionReport struct {
	ID        string
	Processor string
	Status    string
	Reason    string
	Duration  time.Duration
}

// Report separates processing work from generated-file publication.
type Report struct {
	Executed  int
	Reused    int
	Written   int
	Unchanged int
	Removed   int
	Bytes     int
	Actions   []ActionReport
	Duration  time.Duration
}

// Context exposes declared upstream outputs and tracked dependencies to an action.
type Context struct {
	//nolint:containedctx // The processor context deliberately carries cancellation with its tracked access API.
	context.Context

	runner    *runner
	action    Action
	deps      []dependency
	expanding bool
	err       error
	artifacts map[Artifact]bool
}

// ExecutionResource is an opaque identity for one exclusive build resource.
// Share its pointer between actions; create separate tokens for separate owners.
type ExecutionResource struct{ _ byte }

// NewExecutionResource allocates a distinct, nonzero-sized resource identity.
func NewExecutionResource() *ExecutionResource { return &ExecutionResource{} }

// ActionPhase describes an action execution or cache-admission lifecycle event.
type ActionPhase uint8

const (
	// ActionStarted precedes cache lookup and any processor invocation.
	ActionStarted ActionPhase = iota
	// ActionFinished follows cache admission or complete processing, including errors.
	ActionFinished
)

// ActionEvent is optional build-only tracing. Time retains Go's monotonic clock;
// concurrent completion order can differ from the stable Report.Actions order.
type ActionEvent struct {
	Action    string
	Processor string
	Phase     ActionPhase
	Time      time.Time
	Err       error
}
