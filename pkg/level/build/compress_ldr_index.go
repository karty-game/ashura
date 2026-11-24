//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

func nativeTextureIndexActions(
	plan *assetbuild.Plan,
	identity string,
	chunks []assetbuild.Ref,
	pages [][]publishedPage,
	encoded map[string]assetbuild.Ref,
	dependencies []assetbuild.Ref,
	profile NativeTextureOptions,
) ([]assetbuild.Ref, error) {
	var rewritten []assetbuild.Ref

	for index, source := range chunks {
		current := pages[index]

		outputs, err := plan.Add(assetbuild.Action{
			ID: fmt.Sprintf(
				"level/native-texture-index/%s/%d",
				identity,
				index,
			),
			Processor: "level-native-index",
			Revision:  "normalized-ldr-storage@2-wasi1",
			Inputs: append(
				[]assetbuild.Ref{source},
				dependencies...),
			Options: struct{ Storage, Transfer string }{nativeTextureStorage(profile), profile.Transfer},
			Outputs: []assetbuild.Slot{
				{Name: slot, Schema: renderPagesSchema},
			},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return rewriteNativeTexturePages(ctx, current, encoded, profile)
			},
		})
		if err != nil {
			return nil, err
		}

		rewritten = append(rewritten, outputs[0])
	}

	return rewritten, nil
}

func rewriteNativeTexturePages(
	ctx *assetbuild.Context,
	pages []publishedPage,
	encoded map[string]assetbuild.Ref,
	profile NativeTextureOptions,
) (assetbuild.Result, error) {
	if err := applyNativeTextures(ctx, pages, encoded, profile); err != nil {
		return nil, err
	}

	return result(pages)
}

func applyNativeTextures(
	ctx *assetbuild.Context,
	pages []publishedPage,
	encoded map[string]assetbuild.Ref,
	profile NativeTextureOptions,
) error {
	for index := range pages {
		if err := replaceNativePageFiles(ctx, &pages[index], encoded); err != nil {
			return err
		}

		pages[index].Page.Encoding = profile.Transfer
		pages[index].Page.Storage = nativeTextureStorage(profile)
	}

	return nil
}
