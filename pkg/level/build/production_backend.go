//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/ashura/internal/lightmapper"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func (plugin *productionPlugin) planBackend(plan *assetbuild.Plan, identity string) error {
	if plugin.backend.Action != "" || plugin.options.BackendIdentity != "" {
		return nil
	}

	refs, err := plan.Add(assetbuild.Action{
		ID:        "level/production/backend/" + identity,
		Processor: "lightmapper-backend",
		Revision:  "native-adapter@1",
		Cache:     assetbuild.CacheDisabled,
		Outputs:   []assetbuild.Slot{{Name: slot, Schema: "karty.lightmapper-backend@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			options, err := plugin.prepareWorker(ctx)
			if err != nil {
				return nil, err
			}

			identity, err := lightmapper.DetectGPUBackend(ctx, options.SoftwareForCI)
			if err != nil {
				return nil, err
			}

			return assetbuild.Result{slot: []byte(identity)}, nil
		},
	})
	if err != nil {
		return err
	}

	plugin.backend = refs[0]

	return nil
}

func (plugin *productionPlugin) gpuCacheInputs(inputs []assetbuild.Ref) ([]assetbuild.Ref, assetbuild.CachePolicy) {
	if plugin.backend.Action != "" {
		return append(inputs, plugin.backend), assetbuild.CacheAutomatic
	}

	if plugin.options.BackendIdentity != "" {
		return inputs, assetbuild.CacheAutomatic
	}

	return inputs, assetbuild.CacheDisabled
}

func (plugin *productionPlugin) verifyWorkerBackend(ctx *assetbuild.Context) error {
	if plugin.backend.Action == "" {
		return nil
	}

	identity, err := ctx.Input(plugin.backend)
	if err != nil {
		return err
	}

	actual := plugin.worker.Device().Adapter
	if string(identity) != actual {
		return fmt.Errorf("lightmap backend changed from %q to %q: %w", identity, actual, ErrInvalid)
	}

	return nil
}
