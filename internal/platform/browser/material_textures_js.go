//go:build js && wasm

package browser

import (
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
)

func (driver *Driver) standardTextureBindings(entries []any, occlusion, reflection, lightmap *texturedata.Texture) ([]any, error) {
	aoView, err := driver.textureView(occlusion)
	if err != nil {
		return nil, err
	}

	reflectionView, err := driver.textureView(reflection)
	if err != nil {
		return nil, err
	}

	lightmapView, err := driver.textureView(lightmap)
	if err != nil {
		return nil, err
	}

	driver.initLightmapSampler()

	return append(entries,
		jsObject(map[string]any{bindingProperty: render.OcclusionBinding, resourceProperty: aoView}),
		jsObject(map[string]any{bindingProperty: render.ReflectionBinding, resourceProperty: reflectionView}),
		jsObject(map[string]any{bindingProperty: render.LightmapBinding, resourceProperty: lightmapView}),
		jsObject(map[string]any{bindingProperty: render.LightmapSamplerBinding, resourceProperty: driver.lightmapSampler}),
	), nil
}

func (driver *Driver) initLightmapSampler() {
	if driver.lightmapSampler.IsUndefined() {
		driver.lightmapSampler = driver.device.Call(
			"createSampler",
			jsObject(
				map[string]any{
					addressModeUProperty: clampToEdge,
					addressModeVProperty: clampToEdge,
					magFilterProperty:    "linear",
					minFilterProperty:    "linear",
					mipmapFilterProperty: nearestFilter,
				},
			),
		)
	}
}

func (driver *Driver) rnmTextureBindings(entries []any, rnm [3]*texturedata.Texture) ([]any, error) {
	driver.initLightmapSampler()

	for index, image := range rnm {
		if image == nil {
			image = driver.whiteAO
		}

		view, err := driver.textureView(image)
		if err != nil {
			return nil, err
		}

		entries = append(entries, jsObject(map[string]any{bindingProperty: render.RNMLightmapBinding + index, resourceProperty: view}))
	}

	return append(
		entries,
		jsObject(map[string]any{bindingProperty: render.LightmapSamplerBinding, resourceProperty: driver.lightmapSampler}),
	), nil
}
