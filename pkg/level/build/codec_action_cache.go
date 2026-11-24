//go:build !js

package levelbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

// codecActionCache shares equivalent offline encoder producers within one
// variant build. It owns no payload bytes and never bypasses action dependencies,
// tool fingerprints, container admission or publication ownership.
type codecActionCache struct{ refs map[string][]assetbuild.Ref }

func (shared *codecActionCache) add(plan *assetbuild.Plan, action assetbuild.Action) ([]assetbuild.Ref, error) {
	if shared == nil || action.Cache != assetbuild.CacheAutomatic {
		return plan.Add(action)
	}

	key, err := codecActionIdentity(action)
	if err != nil {
		return nil, err
	}

	if refs, found := shared.refs[key]; found {
		return slices.Clone(refs), nil
	}
	// Unlike the requesting profile's name, this identity describes the exact
	// conversion. Reordering RGB/RNM profiles cannot create another encoder job.
	action.ID = "level/shared-codec/" + action.Processor + "/" + key

	refs, err := plan.Add(action)
	if err != nil {
		return nil, err
	}

	if shared.refs == nil {
		shared.refs = make(map[string][]assetbuild.Ref)
	}

	shared.refs[key] = slices.Clone(refs)

	return refs, nil
}

func codecActionIdentity(action assetbuild.Action) (string, error) {
	// Only codec-planning callers use this cache. Their output names/directories
	// are content-addressed constants and do not depend on the requesting profile.
	// Options retain every byte-affecting portable encoder setting.
	encoded, err := json.Marshal(struct {
		Processor string                `json:"processor"`
		Revision  string                `json:"revision"`
		Options   any                   `json:"options"`
		Inputs    []assetbuild.Ref      `json:"inputs"`
		Artifacts []assetbuild.Artifact `json:"artifacts"`
		Outputs   []assetbuild.Slot     `json:"outputs"`
	}{action.Processor, action.Revision, action.Options, action.Inputs, action.Artifacts, action.Outputs})
	if err != nil {
		return "", err
	}

	digest := sha256.Sum256(encoded)

	return hex.EncodeToString(digest[:]), nil
}
