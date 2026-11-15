package engine

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

var (
	errInvalidLightmap          = errors.New("engine: lightmap requires linear irradiance and prepared secondary UVs")
	errTextureOwner             = fmt.Errorf("engine: texture does not belong to this app: %w", ErrOwner)
	errTextureInUse             = fmt.Errorf("engine: texture is referenced by the active scene or compute bindings: %w", ErrInUse)
	errTextureInvalid           = fmt.Errorf("engine: material uses an unloaded or foreign texture: %w", ErrOwner)
	errTextureEncoding          = errors.New("engine: albedo/reflection require SRGB and normal/SSBump/AO maps require Linear textures")
	errInvalidReflectionTexture = errors.New("engine: reflection probe requires an sRGB six-face horizontal cubemap atlas")
	errTooManyLights            = errors.New("engine: standard shading supports at most eight lights")
)

type occlusionMaterialComponent interface {
	RuntimeOcclusion() (*texture.Texture, float32)
}

type texturedMaterialComponent interface {
	RuntimeTextures() (*texture.Texture, *texture.Texture, Vector2, float32)
}

// NewTexture validates and copies GPU-ready bytes. No source-image decoding or
// mip generation occurs here. Backends upload each immutable level on first use.
func (app *App) NewTexture(data texture.Data) (*texture.Texture, error) {
	return app.newTexture(data, texturedata.Create)
}

// AdoptTexture admits immutable GPU-ready pixel storage without copying it.
// The caller relinquishes mutation of every mip's byte storage for the handle's
// lifetime. Mip-slice descriptors are copied; NewTexture retains copy semantics.
// Internal resource loaders can reuse bytes they exclusively own after reading.
func (app *App) AdoptTexture(data texture.Data) (*texture.Texture, error) {
	return app.newTexture(data, texturedata.FromOwned)
}

// CompressedAlbedoFormat returns a supported GPU block format. RGBA8 means
// no compressed albedo target is available; it does not request an RGBA decode.
func (app *App) CompressedAlbedoFormat() texture.Format {
	if app == nil || app.closed {
		return texture.RGBA8
	}

	if driver, ok := app.driver.(interface{ CompressedAlbedoFormat() texturedata.Format }); ok {
		return driver.CompressedAlbedoFormat()
	}

	return texture.RGBA8
}

func (app *App) newTexture(data texture.Data, create func(texture.Data) (*texture.Texture, error)) (*texture.Texture, error) {
	if app == nil || app.closed {
		return nil, errAppClosed
	}

	image, err := create(data)
	if err != nil {
		return nil, err
	}

	if app.textures == nil {
		app.textures = make(map[*texture.Texture]struct{})
	}

	app.textures[image] = struct{}{}

	return image, nil
}

// UnloadTexture releases an owned texture unless the active scene references it.
// Remove its entities or detach their texture fields before unloading.
func (app *App) UnloadTexture(image *texture.Texture) error {
	return app.UnloadTextures([]*texture.Texture{image})
}

// UnloadTextures preflights every handle before releasing any texture. Foreign,
// stale or referenced handles reject the whole batch. Duplicate handles release
// their resource once. This is useful for atomically unloading a model's assets.
func (app *App) UnloadTextures(images []*texture.Texture) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	for _, image := range images {
		if _, owned := app.textures[image]; !owned || !texturedata.Valid(image) {
			return errTextureOwner
		}

		if app.computeTextureInUse(image) {
			return errTextureInUse
		}
	}

	if len(images) == 0 {
		return nil
	}

	candidates := make(map[*texture.Texture]struct{}, min(len(images), len(app.textures)))
	for _, image := range images {
		candidates[image] = struct{}{}
	}

	if app.sceneTexturesInUse(candidates) {
		return errTextureInUse
	}

	for _, image := range images {
		if _, owned := app.textures[image]; !owned {
			continue
		}

		if app.driver != nil {
			app.driver.UnloadTexture(image)
		}

		texturedata.Release(image)
		delete(app.textures, image)
	}

	return nil
}

// Check scene references once for the complete batch, rather than rescanning
// every component for each texture in a multi-texture model.
func (app *App) sceneTexturesInUse(candidates map[*texture.Texture]struct{}) bool {
	inUse := false

	app.scene.Each(func(entity Entity) {
		if inUse {
			return
		}

		app.scene.eachComponent(entity, func(component any) {
			if provider, ok := component.(texturedMaterialComponent); ok {
				albedo, normal, _, _ := provider.RuntimeTextures()
				_, hasAlbedo := candidates[albedo]
				_, hasNormal := candidates[normal]
				inUse = inUse || hasAlbedo || hasNormal
			}

			if provider, ok := component.(occlusionMaterialComponent); ok {
				image, _ := provider.RuntimeOcclusion()
				_, found := candidates[image]
				inUse = inUse || found
			}

			if provider, ok := component.(lightmapMaterialComponent); ok {
				image, _, _ := provider.RuntimeLightmap()
				_, found := candidates[image]
				inUse = inUse || found
			}

			inUse = inUse || rnmTexturesInUse(component, candidates)

			if provider, ok := component.(reflectionProbeComponent); ok {
				image, _, _ := provider.RuntimeReflectionProbe()
				_, found := candidates[image]
				inUse = inUse || found
			}
		})
	})

	return inUse
}

func (app *App) validateTextures(frameTextures ...*texture.Texture) error {
	for _, image := range frameTextures {
		if image == nil {
			continue
		}

		if _, owned := app.textures[image]; !owned || !texturedata.Valid(image) {
			return errTextureInvalid
		}
	}

	return nil
}

func rnmTexturesInUse(component any, candidates map[*texture.Texture]struct{}) bool {
	provider, ok := component.(rnmLightmapMaterialComponent)
	if !ok {
		return false
	}

	maps := provider.RuntimeRNMLightmap()
	if maps == nil {
		return false
	}

	for _, image := range maps.Textures {
		if _, found := candidates[image]; found {
			return true
		}
	}

	return false
}
