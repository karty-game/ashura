// Package impostor loads offline multi-view color/normal/depth captures and
// instantiates their prepared quad. Runtime conversion and atlas rebuilding are
// deliberately absent; the GPU reprojects offline-prepared captures.
package impostor

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

const indicesPerTriangle = 3

var (
	// ErrClosed means the impostor is unloaded or its App has closed.
	ErrClosed = engine.ErrClosed
	// ErrInUse means live entities or bindings prevent unloading. Remove them
	// before retrying. This category is shared with engine.ErrInUse.
	ErrInUse = engine.ErrInUse

	errUnavailable = fmt.Errorf("impostor is unloaded or its App is closed: %w", ErrClosed)
	errInUse       = fmt.Errorf("impostor still has live instances: %w", ErrInUse)
	errWorld       = errors.New("impostor requires a World and finite nonsingular positive-determinant root transform")
	errTextures    = errors.New("impostor texture dimensions, mip counts or encoding do not match prepared metadata")
)

// Asset declares immutable prepared bytes. Source is diagnostic identity, never
// a runtime filesystem path.
type Asset struct {
	source string
	data   []byte
}

// Prepared binds a generated embed to the prepared impostor resource format.
func Prepared(source string, data []byte) Asset { return Asset{source: source, data: data} }

// Impostor owns prepared proxy geometry, static view configuration and two atlas
// textures. Its methods belong to the owning App loop goroutine.
type Impostor struct {
	quality            *qualityState
	app                *engine.App
	document           *impostordata.Document
	geometry           *meshdata.Geometry
	color, normalDepth *texture.Texture
	instances          map[*Instance]struct{}
	loaded             bool
	closeCancel        func()
	proxyTriangles     uint32
}

// Load admits every bounded section before creating App resources. It only
// copies GPU-ready payloads; the supplied quad/view table are not rebuilt.
func (asset Asset) Load(app *engine.App) (*Impostor, error) {
	if app.Closed() {
		return nil, errUnavailable
	}

	document, err := impostordata.Decode(asset.data)
	if err != nil {
		return nil, fmt.Errorf("impostor %s: %w", asset.source, err)
	}

	geometry, err := meshdata.New(document.Vertices, document.Indices)
	if err != nil {
		return nil, fmt.Errorf("impostor quad: %w", err)
	}

	color, err := ktx2.Decode(document.Color)
	if err != nil {
		return nil, fmt.Errorf("impostor color: %w", err)
	}

	normal, err := ktx2.Decode(document.NormalDepth)
	if err != nil {
		return nil, fmt.Errorf("impostor normal/depth: %w", err)
	}

	if !matchingTexture(color, document.Color, document, texture.SRGB) ||
		!matchingTexture(normal, document.NormalDepth, document, texture.Linear) {
		return nil, errTextures
	}

	// Geometry admission bounds the index count below meshdata.MaxIndices.
	//nolint:gosec // The admitted maximum triangle count fits uint32.
	proxyTriangles := uint32(len(document.Indices) / (meshdata.IndexStride * indicesPerTriangle))
	result := &Impostor{app: app, geometry: geometry, loaded: true, instances: make(map[*Instance]struct{}), proxyTriangles: proxyTriangles}
	result.quality = &qualityState{settings: DefaultQuality()}
	result.quality.parameters, _ = qualityParameters(result.quality.settings)

	result.color, err = app.NewTexture(color)
	if err != nil {
		return nil, err
	}

	result.normalDepth, err = app.NewTexture(normal)
	if err != nil {
		_ = app.UnloadTexture(result.color)

		return nil, err
	}
	// Retain only immutable descriptor bytes and metadata. Geometry/textures own
	// their upload copies; the source container remains with its embed owner.
	document.Vertices, document.Indices, document.Color, document.NormalDepth = nil, nil, nil, nil
	document.Uniform = append([]byte(nil), document.Uniform...)
	result.document = &document

	result.closeCancel, err = app.OnClose(func() { _ = result.Unload() })
	if err != nil {
		_ = result.Unload()

		return nil, err
	}

	return result, nil
}

func matchingTexture(data texture.Data, encoded []byte, document impostordata.Document, space texture.ColorSpace) bool {
	format := texture.RGBA8
	if document.Storage == impostordata.StorageBC1 {
		format = texture.BC1

		if !ktx2data.BC1Alpha(encoded) {
			return false
		}
	}

	return data.Width == int(document.AtlasWidth) && data.Height == int(document.AtlasHeight) &&
		len(data.Levels) == int(document.MipLevels) && data.ColorSpace == space && data.Format == format
}

// Source reports the authored model identity retained by the offline baker.
func (value *Impostor) Source() string { return value.document.Source }

// Triangles reports the original model triangle count, before offline capture.
func (value *Impostor) Triangles() uint32 { return value.document.SourceTriangles }

// Center reports the offline-baked object-space center of the source bounds.
func (value *Impostor) Center() engine.Vector3 {
	return engine.Vector3{X: value.document.Center[0], Y: value.document.Center[1], Z: value.document.Center[2]}
}

// ProxyTriangles reports the prepared replacement geometry's triangle count.
func (value *Impostor) ProxyTriangles() uint32 {
	return value.proxyTriangles
}

// Unload releases the asset after its last entity has been removed. App.Close
// also invalidates it and releases all App-owned GPU resources.
func (value *Impostor) Unload() error {
	if value == nil || !value.loaded {
		return nil
	}

	if !value.app.Closed() {
		value.pruneInstances()

		if len(value.instances) > 0 {
			return errInUse
		}

		if err := value.app.UnloadTextures([]*texture.Texture{value.color, value.normalDepth}); err != nil {
			return err
		}
	}

	if value.closeCancel != nil {
		value.closeCancel()
		value.closeCancel = nil
	}

	value.loaded = false
	value.geometry.Release()
	value.geometry = nil
	value.color, value.normalDepth = nil, nil
	value.instances = nil
	value.document.Uniform = nil

	return nil
}

// RuntimePreparedMesh supplies the baked, immutable proxy geometry.
func (value *Impostor) RuntimePreparedMesh() *meshdata.Geometry { return value.geometry }

// RuntimeMeshIdentity allows matching instances to share quad geometry/batches.
func (value *Impostor) RuntimeMeshIdentity() any { return value.geometry }

// RuntimeMeshOwner associates the resource with its App and current lifetime.
func (value *Impostor) RuntimeMeshOwner() (*engine.App, bool) {
	return value.app, value.loaded && !value.app.Closed()
}

// RuntimeMaterial supplies the baked surface roughness and neutral tint.
func (value *Impostor) RuntimeMaterial() (engine.Color, float32) {
	return engine.ColorWhite(), value.document.Roughness
}

// RuntimeTextures supplies sRGB coverage and linear object-normal/depth atlases.
func (value *Impostor) RuntimeTextures() (*texture.Texture, *texture.Texture, engine.Vector2, float32) {
	return value.color, value.normalDepth, engine.Vector2{X: 1, Y: 1}, 0
}

// RuntimeImpostor exposes the immutable prepacked view table to the renderer.
func (value *Impostor) RuntimeImpostor() *impostordata.Document { return value.document }
