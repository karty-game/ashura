// Package shader defines embedded WGSL modules and explicit GPU resource layouts.
// Raw buffer bytes must already follow WGSL alignment; arbitrary Go structs are
// never copied with unsafe. Resources are owned by the App passed to constructors.
package shader

import (
	"fmt"
	"io/fs"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// Module is immutable WGSL source. Device compilation occurs during App.Run setup.
type Module struct{ name, source string }

// WGSL wraps a complete .wgsl file embedded as a string, with its diagnostic path.
func WGSL(name, source string) *Module { return &Module{name: name, source: source} }

// LoadFS reads one explicit WGSL file, typically from go:embed. It does not compile it.
func LoadFS(files fs.FS, path string) (*Module, error) {
	data, err := fs.ReadFile(files, path)
	if err != nil {
		return nil, fmt.Errorf("shader %s: %w", path, err)
	}

	return WGSL(path, string(data)), nil
}

// Name returns the source's diagnostic identity.
func (module *Module) Name() string { return module.name }

// Source returns immutable WGSL text for device compilation.
func (module *Module) Source() string { return module.source }

// Buffer is an App-owned GPU buffer with reusable CPU upload staging.
type Buffer = bufferdata.Buffer

// BufferDesc declares capacity, role and copied initial bytes.
type BufferDesc = bufferdata.Desc

// BufferUsage declares how a buffer can be bound.
type BufferUsage = bufferdata.Usage

const (
	UniformBuffer = bufferdata.Uniform
	StorageBuffer = bufferdata.Storage
)

// BufferOwner is implemented by engine.App; resource constructors need no backend handles.
type BufferOwner interface {
	NewBuffer(desc BufferDesc) (*Buffer, error)
}

// NewBuffer creates a setup-time buffer with an explicit byte descriptor.
func NewBuffer(owner BufferOwner, desc BufferDesc) (*Buffer, error) { return owner.NewBuffer(desc) }

// NewUniformBytes creates a uniform buffer whose size is the supplied byte length.
func NewUniformBytes(owner BufferOwner, data []byte) (*Buffer, error) {
	return owner.NewBuffer(BufferDesc{Label: "uniform", Usage: UniformBuffer, Data: data})
}

// BindingType describes a WGSL binding role.
type BindingType uint8

const (
	Uniform BindingType = iota + 1
	ReadOnlyStorage
	ReadWriteStorage
	StorageTexture
)

// BindingLayout declares one slot in bind group zero. MinSize is bytes for buffers.
type BindingLayout struct {
	Binding uint32
	Type    BindingType
	MinSize uint64
}

// Layout is an explicit, reusable group-zero resource layout.
type Layout []BindingLayout

// Binding connects a slot to one App-owned buffer or storage texture.
type Binding struct {
	Binding uint32
	Buffer  *Buffer
	Texture *texture.Texture
}

// UniformLayout declares a uniform buffer with a minimum WGSL binding size.
func UniformLayout(binding uint32, size uint64) BindingLayout {
	return BindingLayout{Binding: binding, Type: Uniform, MinSize: size}
}

// StorageLayout declares a read/write storage buffer.
func StorageLayout(binding uint32, size uint64) BindingLayout {
	return BindingLayout{Binding: binding, Type: ReadWriteStorage, MinSize: size}
}

// ReadOnlyStorageLayout declares a read-only storage buffer.
func ReadOnlyStorageLayout(binding uint32, size uint64) BindingLayout {
	return BindingLayout{Binding: binding, Type: ReadOnlyStorage, MinSize: size}
}

// StorageTextureLayout declares a write-only rgba8unorm 2D storage texture.
func StorageTextureLayout(binding uint32) BindingLayout {
	return BindingLayout{Binding: binding, Type: StorageTexture}
}

// BindBuffer connects a buffer to its declared slot.
func BindBuffer(binding uint32, buffer *Buffer) Binding {
	return Binding{Binding: binding, Buffer: buffer}
}

// BindTexture connects a GPU-produced texture to its storage slot.
func BindTexture(binding uint32, image *texture.Texture) Binding {
	return Binding{Binding: binding, Texture: image}
}
