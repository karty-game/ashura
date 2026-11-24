//go:build !js

package levelbuild

import (
	"fmt"
	"strconv"

	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func (plugin *productionPlugin) prepareGeometry(ctx *assetbuild.Context, record partitioned, chunk chunkRecord) (assetbuild.Result, error) {
	data, err := ctx.ReadArtifact(chunk.Occluders)
	if err != nil {
		return nil, err
	}

	source, err := bake.SceneFromModel(data, chunk.Description.Space, plugin.options.MetresPerUnit)
	if err != nil {
		return nil, err
	}

	source.ID = record.Source + "/" + chunk.Description.Space

	prepared, err := bake.PrepareScene(source)
	if err != nil {
		return nil, err
	}

	encoded, err := bake.EncodeScene(prepared)
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(encoded)
	if err != nil {
		return nil, err
	}

	return result(productionScene{Data: artifact})
}

type productionLighting struct {
	Emitters    []bake.Emitter
	Environment bake.Environment
}

func (plugin *productionPlugin) lighting(record partitioned, space string) productionLighting {
	lighting := productionLighting{
		Environment: bake.Environment{Sky: bake.Vec3(record.SkyColor), Ground: bake.Vec3(record.GroundColor), LegacyIrradiance: true},
	}
	if plugin.options.Environment != nil {
		lighting.Environment = *plugin.options.Environment
	}

	for _, source := range record.Points {
		if source.Space == space {
			point := source.Point
			lighting.Emitters = append(
				lighting.Emitters,
				bake.Emitter{
					ID:       uint64(len(lighting.Emitters)) + 1,
					Kind:     bake.Point,
					Model:    bake.LegacyRangeV1,
					Position: bake.Vec3(point.Position),
					Range:    point.Range,
					Intensity: bake.Vec3{
						point.Color[0] * point.Intensity,
						point.Color[1] * point.Intensity,
						point.Color[2] * point.Intensity,
					},
				},
			)
		}
	}

	lighting.Emitters = append(lighting.Emitters, plugin.options.Emitters[space]...)

	return lighting
}

func (plugin *productionPlugin) prepareLighting(
	ctx *assetbuild.Context,
	base assetbuild.Ref,
	lighting productionLighting,
) (assetbuild.Result, error) {
	geometry, err := plugin.readScene(ctx, base)
	if err != nil {
		return nil, err
	}

	prepared, err := bake.WithLighting(geometry, lighting.Emitters, lighting.Environment)
	if err != nil {
		return nil, err
	}

	encoded, err := bake.EncodeScene(prepared)
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(encoded)
	if err != nil {
		return nil, err
	}

	return result(productionScene{Data: artifact})
}

func (plugin *productionPlugin) readScene(ctx *assetbuild.Context, ref assetbuild.Ref) (*bake.PreparedScene, error) {
	var index productionScene
	if err := readRecord(ctx, ref, &index); err != nil {
		return nil, err
	}

	// Keep one cached scene. Active callers retain their own immutable pointers
	// when geometry and lit transport overlap; no all-world scene map is retained.
	plugin.sceneMu.Lock()
	defer plugin.sceneMu.Unlock()

	if index.Data.SHA256 == plugin.sceneKey && plugin.scene != nil {
		return plugin.scene, nil
	}

	data, err := ctx.ReadArtifact(index.Data)
	if err != nil {
		return nil, err
	}

	scene, err := bake.DecodeScene(data)
	if err != nil {
		return nil, err
	}

	plugin.sceneKey, plugin.scene = index.Data.SHA256, scene

	return scene, nil
}
func (plugin *productionPlugin) openWorker(ctx *assetbuild.Context) error {
	options, err := plugin.prepareWorker(ctx)
	if err != nil {
		return err
	}

	if plugin.worker == nil {
		plugin.worker, err = bake.NewWorker(ctx.Context, options)
		if err != nil {
			return err
		}
	}

	return plugin.verifyWorkerBackend(ctx)
}

// Track the same native dependencies on every GPU action that opens the worker,
// even when another action has already initialized the binding in this process.
func (plugin *productionPlugin) prepareWorker(ctx *assetbuild.Context) (bake.WorkerOptions, error) {
	library, err := productionNativeLibrary(ctx)
	if err != nil {
		return bake.WorkerOptions{}, err
	}

	if _, err = ctx.TrackLibrary(library); err != nil {
		return bake.WorkerOptions{}, err
	}

	for _, name := range []string{
		"WGPU_NATIVE_PATH", "PATH", "WGPU_BACKEND", "WGPU_ADAPTER_NAME", "WGPU_POWER_PREF", "WGPU_DX12_COMPILER",
		"VK_ICD_FILENAMES", "VK_DRIVER_FILES", "VK_ADD_DRIVER_FILES", "VK_LAYER_PATH", "VK_INSTANCE_LAYERS",
		"VK_LOADER_DRIVERS_SELECT", "VK_LOADER_DRIVERS_DISABLE", "MESA_VK_DEVICE_SELECT", "DRI_PRIME",
		"LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "DYLD_FALLBACK_LIBRARY_PATH",
	} {
		_ = ctx.Getenv(name)
	}

	options := plugin.options.Worker

	if selection := ctx.Getenv("KARTY_LIGHTMAP_SOFTWARE_FOR_CI"); selection != "" {
		software, parseErr := strconv.ParseBool(selection)
		if parseErr != nil {
			return bake.WorkerOptions{}, fmt.Errorf("invalid explicit lightmap software CI selection: %w", parseErr)
		}

		options.SoftwareForCI = options.SoftwareForCI || software
	}

	if err = bake.InitializeNativeLibrary(library); err != nil {
		return bake.WorkerOptions{}, err
	}

	return options, nil
}
