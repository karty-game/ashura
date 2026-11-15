// Package model loads offline-prepared static GLB assets into ordinary ECS
// entities. GLB parsing, node transforms and source-image work are build-only.
package model

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/resourcedata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

var (
	// ErrClosed means the model is unloaded or its owning App has closed.
	// It is the same recovery category as engine.ErrClosed.
	ErrClosed = engine.ErrClosed
	// ErrInUse means live entities or resource bindings prevent unloading.
	// Remove those users before retrying. It is shared with engine.ErrInUse.
	ErrInUse = engine.ErrInUse

	errLightmapLayout   = errors.New("model: missing lightmap layout")
	errLightmapMismatch = errors.New("model: lightmap layout does not match prepared model")
	errUnavailable      = fmt.Errorf("model is unloaded or its owning App is closed: %w", ErrClosed)
	errInUse            = fmt.Errorf("model still has live instance entities: %w", ErrInUse)
	errWorld            = errors.New("model instantiation requires a World")
	errEncoding         = errors.New("prepared model texture role does not match its encoding")
	errPayloadBounds    = errors.New("prepared model exceeds supported byte bounds")
	errRootTransform    = errors.New(
		"model root transform requires finite values and positive scale determinant (all-zero scale selects one)",
	)
)

// Asset explicitly binds a diagnostic source identity to prepared embedded data.
type Asset struct {
	name     string
	data     []byte
	verified *resource.Payload
}

// Prepared declares an offline-prepared model. The name is diagnostic metadata,
// never a runtime filesystem path. Prepared payloads are copied into App-scoped ownership by Load.
func Prepared(name string, data []byte) Asset { return Asset{name: name, data: data} }

// Verified retains an opaque resource-reader result. Its size/checksum and
// immutable CPU ownership are reused; geometry and material admission remain.
// Unlike Prepared, no mutable caller-owned container is retained or copied.
func Verified(name string, payload *resource.Payload) Asset {
	return Asset{name: name, data: resourcedata.Bytes(payload), verified: payload}
}

// Model owns shared prepared primitives and textures for one App.
// Methods belong to the App loop goroutine, like World and texture methods.
type Model struct {
	app         *engine.App
	source      string
	parts       []part
	textures    []*texture.Texture
	instances   map[*Instance]struct{}
	loaded      bool
	closeCancel func()
}

type part struct {
	model    *Model
	geometry *meshdata.Geometry
	material material.Material
}

// Load completely admits the container, all geometry and every prepared texture
// before creating any App resource. It performs no source decoding, transforms,
// geometry repacking, mip generation or GPU waits.
func (asset Asset) Load(app *engine.App) (*Model, error) {
	return asset.load(app, nil)
}

// LightmapLayout supplies the immutable UV streams bound to a prepared model.
// lightmap.Asset implements this interface without making model depend on a baker.
type LightmapLayout interface {
	ModelDigest() [32]byte
	PartCount() int
	PartUV(index int) []byte
}

// LoadLightmapped admits build-generated UV1 streams together with their exact
// remapped model. No mesh or chart generation occurs during loading.
func (asset Asset) LoadLightmapped(app *engine.App, layout LightmapLayout) (*Model, error) {
	if layout == nil {
		return nil, errLightmapLayout
	}

	return asset.load(app, layout)
}

func (asset Asset) load(app *engine.App, layout LightmapLayout) (*Model, error) {
	if app.Closed() {
		return nil, errUnavailable
	}

	if len(asset.data) > modeldata.MaxBytes {
		return nil, fmt.Errorf("model %s (%d-byte maximum): %w", asset.name, modeldata.MaxBytes, errPayloadBounds)
	}

	document, err := modeldata.Decode(asset.data)
	if err != nil {
		return nil, fmt.Errorf("model %s: %w", asset.name, err)
	}

	geometries, images, err := admit(document, resourcedata.Valid(asset.verified))
	if err != nil {
		return nil, fmt.Errorf("model %s: %w", asset.name, err)
	}

	if layout != nil {
		digest := resourcedata.Digest(asset.verified)
		if !resourcedata.Valid(asset.verified) {
			digest = sha256.Sum256(asset.data)
		}

		if layout.ModelDigest() != digest || layout.PartCount() != len(geometries) {
			return nil, errLightmapMismatch
		}

		for index, geometry := range geometries {
			geometries[index], err = geometry.WithLightmapUV(layout.PartUV(index))
			if err != nil {
				return nil, fmt.Errorf("model lightmap primitive %d: %w", index, err)
			}
		}
	}

	result := &Model{app: app, source: document.Source, loaded: true, instances: make(map[*Instance]struct{})}
	if err := result.createTextures(images); err != nil {
		return nil, err
	}

	result.parts = make([]part, len(document.Parts))
	for index, source := range document.Parts {
		result.parts[index] = part{model: result, geometry: geometries[index], material: result.standard(source.Material)}
	}

	cancel, err := app.OnClose(func() { _ = result.Unload() })
	if err != nil {
		_ = result.Unload()

		return nil, err
	}

	result.closeCancel = cancel

	return result, nil
}

func admit(document modeldata.Document, immutable bool) ([]*meshdata.Geometry, []texture.Data, error) {
	geometry := make([]*meshdata.Geometry, len(document.Parts))
	for index, part := range document.Parts {
		create := meshdata.New
		if immutable {
			create = meshdata.FromOwned
		}

		value, err := create(part.Vertices, part.Indices)
		if err != nil {
			return nil, nil, fmt.Errorf("primitive %d (%s): %w", index, part.Name, err)
		}

		geometry[index] = value
	}

	images := make([]texture.Data, len(document.Textures))
	for index, source := range document.Textures {
		data, err := ktx2.Decode(source.Data)
		if err != nil {
			return nil, nil, fmt.Errorf("texture %d: %w", index, err)
		}

		space := texture.Linear
		if source.Role == modeldata.Albedo {
			space = texture.SRGB
		}

		if data.ColorSpace != space {
			return nil, nil, fmt.Errorf("texture %d: %w", index, errEncoding)
		}

		images[index] = data
	}

	return geometry, images, nil
}

// Source returns the original GLB identity recorded by the offline compiler.
func (model *Model) Source() string { return model.source }

// Unload releases the shared model only after every instance entity is removed.
// Texture ownership/references are preflighted as one atomic batch.
func (model *Model) Unload() error {
	if model == nil || !model.loaded {
		return nil
	}

	if !model.app.Closed() {
		model.pruneInstances()

		for instance := range model.instances {
			if instance.Len() != 0 {
				return errInUse
			}
		}

		if err := model.app.UnloadTextures(model.textures); err != nil {
			return err
		}
	}

	if model.closeCancel != nil {
		model.closeCancel()
		model.closeCancel = nil
	}

	model.loaded = false
	for _, source := range model.parts {
		source.geometry.Release()
	}

	model.parts, model.textures, model.instances = nil, nil, nil

	return nil
}

func (model *Model) createTextures(images []texture.Data) error {
	for _, image := range images {
		value, err := model.app.NewTexture(image)
		if err != nil {
			cleanupErr := model.app.UnloadTextures(model.textures)

			return errors.Join(err, cleanupErr)
		}

		model.textures = append(model.textures, value)
	}

	return nil
}

func (model *Model) standard(value modeldata.Material) material.Material {
	return material.Material{
		Color:         engine.Color{R: value.Color[0], G: value.Color[1], B: value.Color[2], A: value.Color[3]},
		CullBackfaces: !value.DoubleSided, Roughness: value.Roughness, NormalStrength: value.NormalStrength, AOStrength: value.AOStrength,
		AlbedoTexture: model.image(value.Albedo), NormalTexture: model.image(value.Normal), AOTexture: model.image(value.AO),
		UVScale:   engine.Vector2{X: 1, Y: 1},
		AlphaMask: value.AlphaMask, AlphaCutoff: value.AlphaCutoff,
	}
}

func (model *Model) image(index int32) *texture.Texture {
	if index < 0 {
		return nil
	}

	return model.textures[index]
}

// RuntimePreparedMesh provides validated, immutable GPU-ready geometry.
func (value part) RuntimePreparedMesh() *meshdata.Geometry {
	if !value.model.loaded || value.model.app.Closed() {
		return nil
	}

	return value.geometry
}

// RuntimeMeshIdentity shares one source primitive across model instances.
func (value part) RuntimeMeshIdentity() any { return value.geometry }

// RuntimeMeshOwner associates prepared geometry with its App and live asset.
func (value part) RuntimeMeshOwner() (*engine.App, bool) {
	return value.model.app, value.model.loaded && !value.model.app.Closed()
}
