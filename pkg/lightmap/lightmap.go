// Package lightmap loads build-prepared UV/visibility inputs and schedules a
// direct-light compute bake. It does not unwrap, rasterize or rebuild geometry.
package lightmap

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// Point is one source in the prepared model's coordinate system. Its attenuation
// is squared (1-distance²/range²), matching live point lighting. Bias zero selects
// .015 world units. Baking is direct diffuse only, with no albedo or ambient fill.
type Point = lightmapkernel.Point

// Asset owns admitted immutable UV, receiver and BVH bytes.
type Asset struct{ document *lightmapdata.Document }

var (
	errOwner    = errors.New("lightmap: nil owner or asset")
	errBaker    = errors.New("lightmap: nil baker")
	errRecorder = errors.New("lightmap: nil recorder")
)

// Declaration names embedded build output without interpreting it as a path.
type Declaration struct {
	name string
	data []byte
}

// Prepared declares prepared inputs; Load verifies and copies their contents.
func Prepared(name string, data []byte) Declaration { return Declaration{name: name, data: data} }

// Load admits all inputs before declaring any App resources.
func (declaration Declaration) Load() (*Asset, error) {
	asset, err := Decode(declaration.data)
	if err != nil {
		return nil, fmt.Errorf("lightmap %s: %w", declaration.name, err)
	}

	return asset, nil
}

// Decode verifies all arrays and references, then takes a private byte copy.
func Decode(data []byte) (*Asset, error) {
	declaration, err := lightmapdata.Decode(data)
	if err != nil {
		return nil, err
	}

	return &Asset{document: declaration}, nil
}

// ModelDigest binds these UV streams to the exact remapped prepared model.
func (asset *Asset) ModelDigest() [32]byte {
	if asset == nil || asset.document == nil {
		return [32]byte{}
	}

	return asset.document.ModelSHA256
}

// PartCount is the number of matching model primitives.
func (asset *Asset) PartCount() int {
	if asset == nil || asset.document == nil {
		return 0
	}

	return len(asset.document.UVs)
}

// PartUV returns immutable packed float32x2 UV1 bytes. Callers must not edit them.
func (asset *Asset) PartUV(index int) []byte {
	if asset == nil || asset.document == nil || index < 0 || index >= len(asset.document.UVs) {
		return nil
	}

	return asset.document.UVs[index]
}

// Size returns the prepared atlas dimensions.
func (asset *Asset) Size() (uint32, uint32) {
	if asset == nil || asset.document == nil {
		return 0, 0
	}

	return asset.document.Width, asset.document.Height
}

// Document exposes immutable preparation bytes to sibling build tools. It must
// not be modified after admission or while a GPU baker refers to it.
func (asset *Asset) Document() *lightmapdata.Document {
	if asset == nil {
		return nil
	}

	return asset.document
}

// Recorder is implemented by engine.Frame; recording performs no GPU wait.
type Recorder interface {
	Dispatch(pipeline *compute.Pipeline, group *compute.BindGroup, x, y, z uint32) error
}

// Baker retains setup-time resources. SetPoint requests work; Record schedules
// a complete bake once. Unchanged frames perform no buffer writes or dispatches.
type Baker struct {
	asset              *Asset
	image              *texture.Texture
	parameters         *shader.Buffer
	pipeline           *compute.Pipeline
	bindings           *compute.BindGroup
	point              Point
	scale              float32
	dirty, initialized bool
	recordings         uint64
}

// New declares one atlas and compute resources before the App starts.
func New(owner compute.Owner, asset *Asset) (*Baker, error) {
	if owner == nil || asset == nil || asset.document == nil {
		return nil, errOwner
	}

	baker := &Baker{asset: asset, scale: 1}

	var err error

	baker.image, err = compute.NewTexture(owner, int(asset.document.Width), int(asset.document.Height))
	if err != nil {
		return nil, err
	}

	baker.parameters, err = shader.NewUniformBytes(owner, make([]byte, lightmapkernel.UniformSize))
	if err != nil {
		return nil, err
	}

	baker.pipeline, err = compute.NewPipeline(owner, lightmapkernel.Description(asset.document))
	if err != nil {
		return nil, err
	}

	bindings := []shader.Binding{shader.BindBuffer(0, baker.parameters)}

	for index, data := range [][]byte{asset.document.Nodes, asset.document.Triangles, asset.document.Receivers, asset.document.Padding} {
		buffer, createErr := compute.NewStorageBytes(owner, data)
		if createErr != nil {
			return nil, createErr
		}

		bindings = append(bindings, shader.BindBuffer(uint32(index+1), buffer))
	}

	bindings = append(bindings, shader.BindTexture(5, baker.image))

	baker.bindings, err = baker.pipeline.BindGroup(bindings...)
	if err != nil {
		return nil, err
	}

	return baker, nil
}

// SetPoint validates settings and requests a bake only if they changed.
func (baker *Baker) SetPoint(point Point) error {
	if baker == nil {
		return errBaker
	}

	parameters, scale, err := lightmapkernel.Parameters(baker.asset.document, point)
	if err != nil {
		return err
	}

	if baker.initialized && point == baker.point {
		return nil
	}

	if err = baker.parameters.Write(0, parameters[:]); err != nil {
		return err
	}

	baker.point, baker.scale, baker.dirty, baker.initialized = point, scale, true, true

	return nil
}

// Texture is the linear rgba8unorm result, decoded by multiplying RGB by Scale.
func (baker *Baker) Texture() *texture.Texture { return baker.image }

// Scale restores irradiance above one without clipping the RGBA8 result.
func (baker *Baker) Scale() float32 { return baker.scale }

// Pending reports whether the next Record will submit a bake.
func (baker *Baker) Pending() bool { return baker.dirty }

// Recordings counts submitted bake dispatches, not GPU completions or timings.
func (baker *Baker) Recordings() uint64 { return baker.recordings }

// Record schedules a dirty bake before rendering and returns immediately.
func (baker *Baker) Record(frame Recorder) error {
	if !baker.dirty {
		return nil
	}

	if frame == nil {
		return errRecorder
	}

	declaration := baker.asset.document
	if err := frame.Dispatch(
		baker.pipeline,
		baker.bindings,
		(declaration.Width+lightmapkernel.WorkgroupSize-1)/lightmapkernel.WorkgroupSize,
		(declaration.Height+lightmapkernel.WorkgroupSize-1)/lightmapkernel.WorkgroupSize,
		1,
	); err != nil {
		return err
	}

	baker.dirty = false
	baker.recordings++

	return nil
}
