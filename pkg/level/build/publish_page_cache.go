//go:build !js

package levelbuild

import (
	"slices"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

// Device families consume identical prepared pages before native texture
// encoding. Share that producer as well as the atlas/codec producers, keeping
// its complete geometry, UV, material, map and origin dependencies in the key.
type packageActionCache struct{ refs map[string][]assetbuild.Ref }

type pagePackagingOptions struct {
	Origin      [3]float32
	Development bool
	Page        pageRecord
}

func (shared *packageActionCache) add(plan *assetbuild.Plan, action assetbuild.Action) ([]assetbuild.Ref, error) {
	if shared == nil {
		return plan.Add(action)
	}

	key, err := codecActionIdentity(action)
	if err != nil {
		return nil, err
	}

	if refs, found := shared.refs[key]; found {
		return slices.Clone(refs), nil
	}

	action.ID = "level/shared-page/" + key

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
