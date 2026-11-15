//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"math"
	"path"
	"slices"
	"strings"
	"time"
)

const defaultMaterializeTimeout = 2 * time.Minute

var errMaterialize = errors.New("invalid Materialize configuration or output")

// MaterializeOptions configures the Materialize v2 CLI's offline surface-map
// generation. BackendIdentity must identify the effective GPU/driver/backend to
// enable caching; an omitted identity executes the tool on every build.
type MaterializeOptions struct {
	Executable      string            `json:"executable"`
	Include         []string          `json:"include,omitempty"`
	Preset          string            `json:"preset"`
	Maps            []string          `json:"maps"`
	NormalStrength  *float64          `json:"normalStrength,omitempty"`
	HeightContrast  *float64          `json:"heightContrast,omitempty"`
	AODepthScale    *float64          `json:"aoDepthScale,omitempty"`
	Seamless        bool              `json:"seamless"`
	Environment     map[string]string `json:"environment,omitempty"`
	BackendIdentity string            `json:"backendIdentity,omitempty"`
	AllowSoftware   bool              `json:"allowSoftware,omitempty"`
	Timeout         time.Duration     `json:"timeout"`
}

type materializePlugin struct{ options MaterializeOptions }

// Materialize preserves selected standalone albedo images and appends generated
// linear normal/AO resources. It never modifies authored sources. By default it
// installs the module-owned checksum-pinned tool for the build host. Executable
// and the tracked MATERIALIZE_CLI environment value provide explicit overrides.
func Materialize(options MaterializeOptions) Plugin { return materializePlugin{options: options} }

func (materializePlugin) Name() string { return materializeToolName }

func (plugin materializePlugin) Plan(plan *Plan) error {
	options, err := normalizeMaterializeOptions(plugin.options)
	if err != nil {
		return err
	}

	reserved := make(map[string]bool)
	for _, resource := range plan.Resources {
		reserved[resource.ID] = true
	}

	inputs := slices.Clone(plan.Resources)
	for _, resource := range inputs {
		selected, matchErr := selectedImage(options.Include, resource.ID)
		if matchErr != nil {
			return matchErr
		}

		if !selected || imageUsage(resource.ID) != UsageAlbedo {
			continue
		}

		applicable, schemaErr := imageResourceApplies(plan, resource.Ref, ImageSetSchema)
		if schemaErr != nil {
			return schemaErr
		}

		if !applicable {
			continue
		}

		if err := addMaterializeAction(plan, resource, options, reserved); err != nil {
			return err
		}
	}

	return nil
}

func addMaterializeAction(plan *Plan, resource Resource, options MaterializeOptions, reserved map[string]bool) error {
	slots := make([]Slot, 0, len(options.Maps))

	identities := make([]string, 0, len(options.Maps))
	for _, role := range options.Maps {
		identity := materializeMapIdentity(resource.ID, role)
		if reserved[identity] {
			return fmt.Errorf("generated resource %q already exists: %w", identity, errMaterialize)
		}

		reserved[identity] = true
		identities = append(identities, identity)
		slots = append(slots, Slot{Name: role, Schema: ImageSetSchema})
	}

	policy := CacheAutomatic
	if options.BackendIdentity == "" {
		policy = CacheDisabled
	}

	refs, err := plan.Add(Action{
		ID: imageActionID(plan, "materialize/"+resource.ID), Processor: "materialize", Revision: "cli-v2@1",
		Options: materializeActionOptions(options), Inputs: []Ref{resource.Ref}, Outputs: slots, Cache: policy,
		Run: func(ctx *Context) (Result, error) {
			return runMaterialize(ctx, resource, options)
		},
	})
	if err != nil {
		return err
	}

	for index, ref := range refs {
		plan.Resources = append(plan.Resources, Resource{ID: identities[index], Ref: ref})
	}

	return nil
}

func normalizeMaterializeOptions(options MaterializeOptions) (MaterializeOptions, error) {
	if options.Preset == "" {
		options.Preset = "default"
	}

	if len(options.Maps) == 0 {
		options.Maps = []string{UsageNormal, UsageAO}
	} else {
		options.Maps = slices.Clone(options.Maps)
	}

	slices.Sort(options.Maps)

	for index, role := range options.Maps {
		if role != UsageNormal && role != UsageAO || index > 0 && role == options.Maps[index-1] {
			return MaterializeOptions{}, fmt.Errorf("only distinct normal/ao maps are supported: %w", errMaterialize)
		}
	}

	if options.Timeout == 0 {
		options.Timeout = defaultMaterializeTimeout
	}

	if options.Timeout < 0 || strings.ContainsAny(options.Preset, "\x00\r\n") {
		return MaterializeOptions{}, errMaterialize
	}

	if err := validateMaterializeOverrides(options); err != nil {
		return MaterializeOptions{}, err
	}

	for _, pattern := range options.Include {
		if _, err := MatchAsset(pattern, ""); err != nil {
			return MaterializeOptions{}, fmt.Errorf("Materialize include %q: %w", pattern, err)
		}
	}

	for name, value := range options.Environment {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, '\x00') {
			return MaterializeOptions{}, fmt.Errorf("invalid tool environment: %w", errMaterialize)
		}
	}

	return options, nil
}

func validateMaterializeOverrides(options MaterializeOptions) error {
	for _, value := range []*float64{options.NormalStrength, options.HeightContrast, options.AODepthScale} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > math.MaxFloat32) {
			return fmt.Errorf("overrides must be finite nonnegative float32 values: %w", errMaterialize)
		}
	}

	return nil
}

func materializeMapIdentity(identity, role string) string {
	directory, base := path.Split(identity)
	if strings.EqualFold(base, UsageAlbedo) {
		return directory + role
	}

	for _, suffix := range []string{"-albedo", "_albedo"} {
		if strings.HasSuffix(strings.ToLower(identity), suffix) {
			return identity[:len(identity)-len(UsageAlbedo)] + role
		}
	}

	return identity + "-" + role
}
