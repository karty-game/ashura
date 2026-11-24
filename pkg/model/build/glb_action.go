//go:build !js

package modelbuild

import (
	compiler "github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func compileModelAction(
	ctx *assetbuild.Context,
	source, name string,
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

	prepared := "model_" + name + ".kmodel"

	code, codeErr := catalog(name, source, prepared)
	if codeErr != nil {
		return nil, codeErr
	}

	files := assetbuild.Bundle{"models/" + prepared: result.Data, "models/model_" + name + ".go": code}

	bundle, marshalErr := assetbuild.EncodeBundle(files)
	if marshalErr != nil {
		return nil, marshalErr
	}

	return assetbuild.Result{bundleSlot: bundle, PreparedSlot: result.Data}, nil
}
