//go:build !js

package modelbuild

import (
	compiler "github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func compileModelAction(
	ctx *assetbuild.Context,
	source, name string,
	lightmap *LightmapOptions,
	options GLBOptions,
) (assetbuild.Result, error) {
	data, readErr := ctx.ReadFile(source)
	if readErr != nil {
		return nil, readErr
	}

	result, compileErr := compiler.CompileWithOptions(data, source, compiler.Options{MaxTextureDimension: options.MaxTextureDimension})
	if compileErr != nil {
		return nil, compileErr
	}

	var lightmapData []byte

	if lightmap != nil {
		prepared, prepareErr := prepareLightmap(ctx, result.Data, *lightmap)
		if prepareErr != nil {
			return nil, prepareErr
		}

		result.Data, lightmapData = prepared.Model, prepared.Lightmap
	}

	prepared := "model_" + name + ".kmodel"

	code, codeErr := catalog(name, source, prepared)
	if codeErr != nil {
		return nil, codeErr
	}

	files := assetbuild.Bundle{"models/" + prepared: result.Data, "models/model_" + name + ".go": code}
	if lightmap != nil {
		declaration, declarationErr := lightmapCatalog(name, source, "lightmap_"+name+".klightmap")
		if declarationErr != nil {
			return nil, declarationErr
		}

		files["models/lightmap_"+name+".go"] = declaration
		files["models/lightmap_"+name+".klightmap"] = lightmapData
	}

	bundle, marshalErr := assetbuild.EncodeBundle(files)
	if marshalErr != nil {
		return nil, marshalErr
	}

	outputs := assetbuild.Result{bundleSlot: bundle, PreparedSlot: result.Data}
	if lightmap != nil {
		outputs[LightmapSlot] = lightmapData
	}

	return outputs, nil
}
